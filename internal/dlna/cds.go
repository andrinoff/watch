package dlna

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
)

// Entry is one thing a television can browse to: a folder or a video.
type Entry struct {
	ID       string
	ParentID string
	Title    string
	// "" for folders, "object.item.videoItem" for videos.
	Class      string
	ChildCount int
	// Items only: the URL the renderer plays, relative to this server.
	URI          string
	ProtocolInfo string
	Mime         string
	Size         int64
	Duration     string // "1:52:05" or ""
	Resolution   string // "1920x1080" or ""
}

// Tree is what the ContentDirectory serves. The library implements it.
type Tree interface {
	// Root returns the top of the shelf.
	Root() []Entry
	// Children of an object, or ok=false when the id is unknown.
	Children(objectID string) (entries []Entry, total int, ok bool)
	// Metadata for BrowseMetadata of a single object.
	Metadata(objectID string) (entry Entry, ok bool)
}

// DeviceXML describes the MediaServer a renderer fetches right after SSDP.
// Base is the URL the renderer used, so every URL inside is absolute.
func DeviceXML(base, uuid, friendlyName string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <device>
    <deviceType>urn:schemas-upnp-org:device:MediaServer:1</deviceType>
    <friendlyName>%s</friendlyName>
    <manufacturer>watch</manufacturer>
    <manufacturerURL>https://andrinoff.com</manufacturerURL>
    <modelDescription>Movies from %s</modelDescription>
    <modelName>watch</modelName>
    <modelNumber>1.0</modelNumber>
    <UDN>uuid:%s</UDN>
    <iconList>
      <icon>
        <mimetype>image/png</mimetype>
        <width>240</width><height>240</height>
        <depth>24</depth>
        <url>%s/dlna/icon.png</url>
      </icon>
    </iconList>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:ContentDirectory:1</serviceType>
        <serviceId>urn:upnp-org:serviceId:ContentDirectory</serviceId>
        <SCPDURL>%s/dlna/cds.xml</SCPDURL>
        <controlURL>%s/dlna/cds/control</controlURL>
        <eventSubURL>%s/dlna/cds/events</eventSubURL>
      </service>
      <service>
        <serviceType>urn:schemas-upnp-org:service:ConnectionManager:1</serviceType>
        <serviceId>urn:upnp-org:serviceId:ConnectionManager</serviceId>
        <SCPDURL>%s/dlna/cm.xml</SCPDURL>
        <controlURL>%s/dlna/cm/control</controlURL>
        <eventSubURL>%s/dlna/cm/events</eventSubURL>
      </service>
    </serviceList>
  </device>
</root>`, xmlEsc(friendlyName), xmlEsc(friendlyName), uuid, base, base, base, base, base, base, base)
}

// The ConnectionManager description: three trivial actions renderers call.
func ConnectionManagerXML() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<scpd xmlns="urn:schemas-upnp-org:service-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <actionList>
    <action><name>GetProtocolInfo</name><argumentList>
      <argument><name>Source</name><direction>out</direction><relatedStateVariable>SourceProtocolInfo</relatedStateVariable></argument>
      <argument><name>Sink</name><direction>out</direction><relatedStateVariable>SinkProtocolInfo</relatedStateVariable></argument>
    </argumentList></action>
    <action><name>GetCurrentConnectionIDs</name><argumentList>
      <argument><name>ConnectionIDs</name><direction>out</direction><relatedStateVariable>CurrentConnectionIDs</relatedStateVariable></argument>
    </argumentList></action>
    <action><name>GetCurrentConnectionInfo</name><argumentList>
      <argument><name>ConnectionID</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_ConnectionID</relatedStateVariable></argument>
      <argument><name>RcsID</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_RcsID</relatedStateVariable></argument>
      <argument><name>AVTransportID</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_AVTransportID</relatedStateVariable></argument>
      <argument><name>ProtocolInfo</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_ProtocolInfo</relatedStateVariable></argument>
      <argument><name>PeerConnectionManager</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_ConnectionManager</relatedStateVariable></argument>
      <argument><name>PeerConnectionID</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_ConnectionID</relatedStateVariable></argument>
      <argument><name>Direction</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Direction</relatedStateVariable></argument>
      <argument><name>Status</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_ConnectionStatus</relatedStateVariable></argument>
    </argumentList></action>
  </actionList>
  <serviceStateTable>
    <stateVariable sendEvents="yes"><name>SourceProtocolInfo</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="yes"><name>SinkProtocolInfo</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="yes"><name>CurrentConnectionIDs</name><dataType>string</dataType></stateVariable>
    <stateVariable><name>A_ARG_TYPE_ConnectionID</name><dataType>i4</dataType></stateVariable>
    <stateVariable><name>A_ARG_TYPE_RcsID</name><dataType>i4</dataType></stateVariable>
    <stateVariable><name>A_ARG_TYPE_AVTransportID</name><dataType>i4</dataType></stateVariable>
    <stateVariable><name>A_ARG_TYPE_ProtocolInfo</name><dataType>string</dataType></stateVariable>
    <stateVariable><name>A_ARG_TYPE_ConnectionManager</name><dataType>string</dataType></stateVariable>
    <stateVariable><name>A_ARG_TYPE_Direction</name><dataType>string</dataType></stateVariable>
    <stateVariable><name>A_ARG_TYPE_ConnectionStatus</name><dataType>string</dataType></stateVariable>
  </serviceStateTable>
</scpd>`
}

// The ContentDirectory description: everything a renderer needs to browse.
func ContentDirectoryXML() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<scpd xmlns="urn:schemas-upnp-org:service-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <actionList>
    <action><name>GetSearchCapabilities</name><argumentList>
      <argument><name>SearchCaps</name><direction>out</direction><relatedStateVariable>SearchCapabilities</relatedStateVariable></argument>
    </argumentList></action>
    <action><name>GetSortCapabilities</name><argumentList>
      <argument><name>SortCaps</name><direction>out</direction><relatedStateVariable>SortCapabilities</relatedStateVariable></argument>
    </argumentList></action>
    <action><name>GetSystemUpdateID</name><argumentList>
      <argument><name>Id</name><direction>out</direction><relatedStateVariable>SystemUpdateID</relatedStateVariable></argument>
    </argumentList></action>
    <action><name>Browse</name><argumentList>
      <argument><name>ObjectID</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_ObjectID</relatedStateVariable></argument>
      <argument><name>BrowseFlag</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_BrowseFlag</relatedStateVariable></argument>
      <argument><name>Filter</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_Filter</relatedStateVariable></argument>
      <argument><name>StartingIndex</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_Index</relatedStateVariable></argument>
      <argument><name>RequestedCount</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable></argument>
      <argument><name>SortCriteria</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_SortCriteria</relatedStateVariable></argument>
      <argument><name>Result</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Result</relatedStateVariable></argument>
      <argument><name>NumberReturned</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable></argument>
      <argument><name>TotalMatches</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable></argument>
      <argument><name>UpdateID</name><direction>out</direction><relatedStateVariable>SystemUpdateID</relatedStateVariable></argument>
    </argumentList></action>
  </actionList>
  <serviceStateTable>
    <stateVariable sendEvents="yes"><name>SystemUpdateID</name><dataType>ui4</dataType></stateVariable>
    <stateVariable sendEvents="yes"><name>ContainerUpdateIDs</name><dataType>string</dataType></stateVariable>
    <stateVariable><name>SearchCapabilities</name><dataType>string</dataType></stateVariable>
    <stateVariable><name>SortCapabilities</name><dataType>string</dataType></stateVariable>
    <stateVariable><name>A_ARG_TYPE_ObjectID</name><dataType>string</dataType></stateVariable>
    <stateVariable><name>A_ARG_TYPE_BrowseFlag</name><dataType>string</dataType><allowedValueList>
      <allowedValue>BrowseMetadata</allowedValue><allowedValue>BrowseDirectChildren</allowedValue>
    </allowedValueList></stateVariable>
    <stateVariable><name>A_ARG_TYPE_Filter</name><dataType>string</dataType></stateVariable>
    <stateVariable><name>A_ARG_TYPE_Index</name><dataType>ui4</dataType></stateVariable>
    <stateVariable><name>A_ARG_TYPE_Count</name><dataType>ui4</dataType></stateVariable>
    <stateVariable><name>A_ARG_TYPE_SortCriteria</name><dataType>string</dataType></stateVariable>
    <stateVariable><name>A_ARG_TYPE_Result</name><dataType>string</dataType></stateVariable>
  </serviceStateTable>
</scpd>`
}

// DIDL renders one entry in the DIDL-Lite dialect renderers expect. URIs are
// made absolute with base, which comes from the request's own Host.
func DIDL(entries []Entry, base string) string {
	var b strings.Builder
	b.WriteString(`<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" `)
	b.WriteString(`xmlns:dc="http://purl.org/dc/elements/1.1/" `)
	b.WriteString(`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">`)
	for _, entry := range entries {
		if entry.Class == "" {
			b.WriteString(fmt.Sprintf(`<container id="%s" parentID="%s" childCount="%d" restricted="1">`,
				xmlEsc(entry.ID), xmlEsc(entry.ParentID), entry.ChildCount))
			b.WriteString(fmt.Sprintf(`<dc:title>%s</dc:title>`, xmlEsc(entry.Title)))
			b.WriteString(`<upnp:class>object.container.storageFolder</upnp:class>`)
			b.WriteString(`</container>`)
			continue
		}
		b.WriteString(fmt.Sprintf(`<item id="%s" parentID="%s" restricted="1">`,
			xmlEsc(entry.ID), xmlEsc(entry.ParentID)))
		b.WriteString(fmt.Sprintf(`<dc:title>%s</dc:title>`, xmlEsc(entry.Title)))
		b.WriteString(`<upnp:class>object.item.videoItem</upnp:class>`)
		res := fmt.Sprintf(`<res protocolInfo="%s" size="%d"`, xmlEsc(entry.ProtocolInfo), entry.Size)
		if entry.Duration != "" {
			res += fmt.Sprintf(` duration="%s"`, xmlEsc(entry.Duration))
		}
		if entry.Resolution != "" {
			res += fmt.Sprintf(` resolution="%s"`, xmlEsc(entry.Resolution))
		}
		res += fmt.Sprintf(">%s/%s</res>", strings.TrimSuffix(base, "/"), strings.TrimPrefix(entry.URI, "/"))
		b.WriteString(res)
		b.WriteString(`</item>`)
	}
	b.WriteString(`</DIDL-Lite>`)
	return b.String()
}

// ProtocolInfo builds the DLNA transfer string for a mime type. OP=01
// announces byte-range support, which every renderer asks for.
func ProtocolInfo(mime string) string {
	if mime == "video/mp4" {
		return "http-get:*:video/mp4:DLNA.ORG_PN=AVC_MP4_EU;DLNA.ORG_OP=01;DLNA.ORG_CI=0"
	}
	return "http-get:*:" + mime + ":DLNA.ORG_OP=01;DLNA.ORG_CI=0"
}

// FormatDuration turns seconds into the DLNA shape, "1:52:05".
func FormatDuration(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	total := int(seconds)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%d:%02d:%02d", h, m, s)
}

// --- SOAP ---

// Control parses a SOAP request and answers with SOAP. It is the whole
// ContentDirectory conversation; tree is where entries come from.
func Control(body []byte, tree Tree, base string) (action string, response string, code int) {
	action, args := parseSOAPAction(body)
	switch action {
	case "Browse":
		return "Browse", browse(args, tree, base), http.StatusOK
	case "GetSearchCapabilities":
		return "GetSearchCapabilities", soapEnvelope(`<SearchCaps></SearchCaps>`), http.StatusOK
	case "GetSortCapabilities":
		return "GetSortCapabilities", soapEnvelope(`<SortCaps></SortCaps>`), http.StatusOK
	case "GetSystemUpdateID":
		return "GetSystemUpdateID", soapEnvelope(`<Id>1</Id>`), http.StatusOK
	case "":
		return "", soapFault(402, "Invalid Args"), http.StatusBadRequest
	default:
		return action, soapFault(401, "Invalid Action"), http.StatusOK
	}
}

func browse(args map[string]string, tree Tree, base string) string {
	objectID := args["ObjectID"]
	flag := args["BrowseFlag"]

	if flag == "BrowseMetadata" {
		if entry, ok := tree.Metadata(objectID); ok {
			didl := DIDL([]Entry{entry}, base)
			return browseEnvelope(didl, 1, 1)
		}
		return browseEnvelope(DIDL(nil, base), 0, 0)
	}

	entries := []Entry{}
	total := 0
	if objectID == "0" {
		entries = tree.Root()
		total = len(entries)
	} else if children, count, ok := tree.Children(objectID); ok {
		entries = children
		total = count
	}

	start, count := window(args, len(entries))
	end := len(entries)
	if count > 0 && start+count < end {
		end = start + count
	}
	if start > len(entries) {
		start = len(entries)
	}
	if start > end {
		end = start
	}
	return browseEnvelope(DIDL(entries[start:end], base), end-start, total)
}

// window reads StartingIndex and RequestedCount with the defaults renderers
// rely on: 0 means "from the start" and "everything".
func window(args map[string]string, total int) (start, count int) {
	if parsed, ok := atoi(args["StartingIndex"]); ok {
		start = parsed
	}
	if parsed, ok := atoi(args["RequestedCount"]); ok && parsed > 0 {
		count = parsed
	}
	return start, count
}

func atoi(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	value := 0
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		value = value*10 + int(ch-'0')
	}
	return value, true
}

func browseEnvelope(didl string, returned, total int) string {
	// DIDL contains angle brackets, so it is escaped as a SOAP string
	// argument, which is exactly how every server ships it.
	escaped := xmlEsc(didl)
	body := fmt.Sprintf(
		`<u:BrowseResponse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">`+
			`<Result>%s</Result><NumberReturned>%d</NumberReturned><TotalMatches>%d</TotalMatches><UpdateID>1</UpdateID>`+
			`</u:BrowseResponse>`, escaped, returned, total)
	return soapEnvelope(body)
}

func soapEnvelope(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">` +
		`<s:Body>` + body + `</s:Body></s:Envelope>`
}

func soapFault(code int, description string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">` +
		`<s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>UPnPError</faultstring>` +
		fmt.Sprintf(`<detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0"><errorCode>%d</errorCode><errorDescription>%s</errorDescription></UPnPError></detail>`,
			code, xmlEsc(description)) +
		`</s:Fault></s:Body></s:Envelope>`
}

// parseSOAPAction walks the envelope ignoring namespace prefixes, because
// renderers all invent their own.
func parseSOAPAction(body []byte) (string, map[string]string) {
	decoder := xml.NewDecoder(strings.NewReader(string(body)))
	args := map[string]string{}
	action := ""
	depth := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch element := token.(type) {
		case xml.StartElement:
			depth++
			// The action is the first element inside Body.
			if action == "" && depth >= 3 {
				action = element.Name.Local
				continue
			}
			if action != "" && depth >= 4 {
				var value string
				if err := decoder.DecodeElement(&value, &element); err == nil {
					args[element.Name.Local] = value
				}
			}
		case xml.EndElement:
			depth--
		}
	}
	return action, args
}

func xmlEsc(value string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}
