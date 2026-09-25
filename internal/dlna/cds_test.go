package dlna

import (
	"encoding/xml"
	"strings"
	"testing"
)

type fakeTree struct {
	entries []Entry
	meta    Entry
}

func (f fakeTree) Root() []Entry { return f.entries }
func (f fakeTree) Children(objectID string) ([]Entry, int, bool) {
	return f.entries, len(f.entries), true
}
func (f fakeTree) Metadata(objectID string) (Entry, bool) { return f.meta, true }

var testTree = fakeTree{
	entries: []Entry{
		{ID: "dabc", ParentID: "0", Title: "Trailers", ChildCount: 1},
		{ID: "vdef", ParentID: "0", Title: "The Neon Archive", Class: "object.item.videoItem",
			URI: "/dlna/media/def.mp4", ProtocolInfo: ProtocolInfo("video/mp4"), Mime: "video/mp4",
			Size: 8678251, Duration: "1:52:05", Resolution: "1920x1080"},
	},
	meta: Entry{ID: "0", Title: "Movies", ChildCount: 2},
}

const browseSOAP = `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body><u:Browse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1"><ObjectID>0</ObjectID><BrowseFlag>BrowseDirectChildren</BrowseFlag><Filter>*</Filter><StartingIndex>0</StartingIndex><RequestedCount>0</RequestedCount><SortCriteria></SortCriteria></u:Browse></s:Body></s:Envelope>`

// parse the Browse response back into its Result as real XML.
func unpackBrowse(t *testing.T, soap string) (didl string, returned, total string) {
	t.Helper()
	var envelope struct {
		Body struct {
			Response struct {
				Result         string `xml:"Result"`
				NumberReturned string `xml:"NumberReturned"`
				TotalMatches   string `xml:"TotalMatches"`
			} `xml:"BrowseResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal([]byte(soap), &envelope); err != nil {
		t.Fatalf("the SOAP answer itself should be valid XML: %v\n%s", err, soap)
	}
	return envelope.Body.Response.Result, envelope.Body.Response.NumberReturned, envelope.Body.Response.TotalMatches
}

func unpackDIDL(t *testing.T, raw string) []struct{ ID, Title string } {
	t.Helper()
	var didl struct {
		Containers []struct {
			ID    string `xml:"id,attr"`
			Title string `xml:"http://purl.org/dc/elements/1.1/ title"`
		} `xml:"container"`
		Items []struct {
			ID    string `xml:"id,attr"`
			Title string `xml:"http://purl.org/dc/elements/1.1/ title"`
			Res   struct {
				Info string `xml:"protocolInfo,attr"`
				URI  string `xml:",chardata"`
			} `xml:"res"`
		} `xml:"item"`
	}
	if err := xml.Unmarshal([]byte(raw), &didl); err != nil {
		t.Fatalf("the DIDL must be valid XML: %v\n%s", err, raw)
	}
	var out []struct{ ID, Title string }
	for _, c := range didl.Containers {
		out = append(out, struct{ ID, Title string }{c.ID, c.Title})
	}
	for _, item := range didl.Items {
		out = append(out, struct{ ID, Title string }{item.ID, item.Title})
		if !strings.HasPrefix(item.Res.URI, "http://test/dlna/media/") {
			t.Errorf("res URI = %q, want an absolute URL on this host", item.Res.URI)
		}
		if item.Res.Info == "" {
			t.Errorf("res has no protocolInfo")
		}
	}
	return out
}

func TestBrowseRootReturnsFoldersAndItems(t *testing.T) {
	action, response, status := Control([]byte(browseSOAP), testTree, "http://test")
	if action != "Browse" || status != 200 {
		t.Fatalf("action=%q status=%d", action, status)
	}
	didl, returned, total := unpackBrowse(t, response)
	if returned != "2" || total != "2" {
		t.Errorf("returned=%s total=%s, want 2/2", returned, total)
	}
	entries := unpackDIDL(t, didl)
	if len(entries) != 2 || entries[0].Title != "Trailers" || entries[1].Title != "The Neon Archive" {
		t.Errorf("entries = %#v", entries)
	}
}

func TestBrowseWindowsTheAnswer(t *testing.T) {
	withWindow := strings.Replace(browseSOAP, "<StartingIndex>0</StartingIndex><RequestedCount>0</RequestedCount>",
		"<StartingIndex>1</StartingIndex><RequestedCount>1</RequestedCount>", 1)
	_, response, _ := Control([]byte(withWindow), testTree, "http://test")
	didl, returned, total := unpackBrowse(t, response)
	if returned != "1" || total != "2" {
		t.Errorf("returned=%s total=%s, want 1/2", returned, total)
	}
	entries := unpackDIDL(t, didl)
	if len(entries) != 1 || entries[0].Title != "The Neon Archive" {
		t.Errorf("entries = %#v", entries)
	}
}

func TestBrowseMetadata(t *testing.T) {
	withMeta := strings.Replace(browseSOAP, "<BrowseFlag>BrowseDirectChildren</BrowseFlag>", "<BrowseFlag>BrowseMetadata</BrowseFlag>", 1)
	_, response, _ := Control([]byte(withMeta), testTree, "http://test")
	didl, returned, total := unpackBrowse(t, response)
	if returned != "1" || total != "1" {
		t.Errorf("returned=%s total=%s", returned, total)
	}
	entries := unpackDIDL(t, didl)
	if len(entries) != 1 || entries[0].ID != "0" || entries[0].Title != "Movies" {
		t.Errorf("entries = %#v", entries)
	}
}

func TestUnknownActionIsAFault(t *testing.T) {
	body := []byte(strings.Replace(browseSOAP, "<u:Browse xmlns:u=\"urn:schemas-upnp-org:service:ContentDirectory:1\">",
		"<u:Nonsense xmlns:u=\"urn:schemas-upnp-org:service:ContentDirectory:1\">", 1))
	action, response, _ := Control(body, testTree, "http://test")
	if action != "Nonsense" {
		t.Errorf("action = %q", action)
	}
	if !strings.Contains(response, "401") || !strings.Contains(response, "Invalid Action") {
		t.Errorf("expected a SOAP fault, got %s", response)
	}
}

func TestDIDLItemShape(t *testing.T) {
	raw := DIDL([]Entry{testTree.entries[1]}, "http://test")
	entries := unpackDIDL(t, raw)
	if len(entries) != 1 {
		t.Fatalf("entries = %#v", entries)
	}
	if !strings.Contains(raw, `duration="1:52:05"`) || !strings.Contains(raw, `resolution="1920x1080"`) {
		t.Errorf("metadata missing from the DIDL: %s", raw)
	}
	if !strings.Contains(raw, `object.item.videoItem`) {
		t.Errorf("class missing: %s", raw)
	}
}

func TestProtocolInfoShapes(t *testing.T) {
	if got := ProtocolInfo("video/mp4"); !strings.Contains(got, "AVC_MP4_EU") {
		t.Errorf("mp4 protocolInfo = %q", got)
	}
	if got := ProtocolInfo("video/x-matroska"); !strings.Contains(got, "DLNA.ORG_OP=01") {
		t.Errorf("mkv protocolInfo = %q", got)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[float64]string{0: "", 45: "0:00:45", 65: "0:01:05", 6725: "1:52:05"}
	for in, want := range cases {
		if got := FormatDuration(in); got != want {
			t.Errorf("FormatDuration(%v) = %q, want %q", in, got, want)
		}
	}
}
