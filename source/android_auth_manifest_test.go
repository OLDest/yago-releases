package main

import (
	"encoding/xml"
	"io"
	"os"
	"testing"
	"yago/internal/updates"
)

// Verify the manifest contract used by the packaged auth handoff: a private
// process/activity and signature-only IPC, rather than an exported token receiver.
func TestAndroidAuthManifestIsolationAndVersion(t *testing.T) {
	f, e := os.Open("android/AndroidManifest.xml")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	d := xml.NewDecoder(f)
	found, permission, usesPermission, version := false, false, false, false
	for {
		token, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		s, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		attrs := map[string]string{}
		for _, a := range s.Attr {
			attrs[a.Name.Local] = a.Value
		}
		switch s.Name.Local {
		case "manifest":
			version = attrs["versionName"] == updates.Version && attrs["versionCode"] == "35"
		case "permission":
			if attrs["name"] == "com.yago.mobile.INTERNAL_AUTH" {
				permission = attrs["protectionLevel"] == "signature"
			}
		case "uses-permission":
			if attrs["name"] == "com.yago.mobile.INTERNAL_AUTH" {
				usesPermission = true
			}
		case "activity":
			if attrs["name"] == "com.yago.mobile.ZvukAuthActivity" {
				found = attrs["exported"] == "false" && attrs["process"] == ":zvuk_auth"
			}
		}
	}
	if !found || !permission || !usesPermission || !version {
		t.Fatal("Android login packaging contract", found, permission, usesPermission, version)
	}
}
