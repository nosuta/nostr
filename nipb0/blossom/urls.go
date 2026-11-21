package blossom

import "strings"

// UploadURL is the BUD-02 upload endpoint for a server base URL. A trailing
// slash is tolerated.
func UploadURL(server string) string {
	return strings.TrimRight(server, "/") + "/upload"
}
