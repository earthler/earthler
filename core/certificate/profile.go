package certificate

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"fmt"
)

// ProfileText is what a phone shows while it installs the profile. The
// screen fills it in the community's language.
type ProfileText struct {
	Name         string // title of the profile, such as the community's name
	Organization string // who made it, shown under the title
	Description  string // what the certificate is for and what it cannot do
}

// Profile wraps the root in an Apple configuration profile. An iPhone offers
// to install a certificate it receives this way, with the name and the
// explanation written here, instead of a bare file it does not know what to
// do with. The identifiers come from the root itself, so installing the
// same profile twice replaces it instead of adding a second copy.
func (c *Chain) Profile(text ProfileText) []byte {
	sum := sha256.Sum256(c.root.Raw)
	id := "org.earthler." + c.domain
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	b.WriteString("\t<key>PayloadContent</key>\n\t<array>\n\t\t<dict>\n")
	entry(&b, "\t\t\t", "PayloadCertificateFileName", c.domain+".cer")
	b.WriteString("\t\t\t<key>PayloadContent</key>\n\t\t\t<data>" + base64.StdEncoding.EncodeToString(c.root.Raw) + "</data>\n")
	entry(&b, "\t\t\t", "PayloadDescription", text.Description)
	entry(&b, "\t\t\t", "PayloadDisplayName", c.root.Subject.CommonName)
	entry(&b, "\t\t\t", "PayloadIdentifier", id+".root")
	entry(&b, "\t\t\t", "PayloadType", "com.apple.security.root")
	entry(&b, "\t\t\t", "PayloadUUID", uuid(sum[:16]))
	b.WriteString("\t\t\t<key>PayloadVersion</key>\n\t\t\t<integer>1</integer>\n")
	b.WriteString("\t\t</dict>\n\t</array>\n")
	entry(&b, "\t", "PayloadDescription", text.Description)
	entry(&b, "\t", "PayloadDisplayName", text.Name)
	entry(&b, "\t", "PayloadIdentifier", id)
	entry(&b, "\t", "PayloadOrganization", text.Organization)
	b.WriteString("\t<key>PayloadRemovalDisallowed</key>\n\t<false/>\n")
	entry(&b, "\t", "PayloadType", "Configuration")
	entry(&b, "\t", "PayloadUUID", uuid(sum[16:]))
	b.WriteString("\t<key>PayloadVersion</key>\n\t<integer>1</integer>\n")
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes()
}

// entry writes one key and its text, escaped for XML.
func entry(b *bytes.Buffer, indent, key, value string) {
	b.WriteString(indent + "<key>" + key + "</key>\n" + indent + "<string>")
	xml.EscapeText(b, []byte(value))
	b.WriteString("</string>\n")
}

// uuid writes 16 bytes in the form of a version 4 UUID.
func uuid(b []byte) string {
	u := make([]byte, 16)
	copy(u, b)
	u[6] = u[6]&0x0f | 0x40
	u[8] = u[8]&0x3f | 0x80
	return fmt.Sprintf("%X-%X-%X-%X-%X", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}
