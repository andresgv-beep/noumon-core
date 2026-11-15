package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func makeDepDeb(t *testing.T, member string, body []byte) []byte {
	t.Helper()
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	archive := tar.NewWriter(gz)
	if err := archive.WriteHeader(&tar.Header{Name: member, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	var deb bytes.Buffer
	deb.WriteString("!<arch>\n")
	for _, entry := range []struct {
		name string
		data []byte
	}{{"debian-binary", []byte("2.0\n")}, {"control.tar.gz", []byte("ignored")}, {"data.tar.gz", data.Bytes()}} {
		fmt.Fprintf(&deb, "%-16s%-12d%-6d%-6d%-8s%-10d`\n", entry.name, 0, 0, 0, "100644", len(entry.data))
		deb.Write(entry.data)
		if len(entry.data)%2 != 0 {
			deb.WriteByte('\n')
		}
	}
	return deb.Bytes()
}

func TestDependencyPlatformsKeepWindowsAndAddLinuxAMD64(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		manifest := depManifestFor(goos, "amd64")
		if len(manifest) != 1 || manifest[0].Bytes <= 0 || len(manifest[0].SHA256) != 64 || !strings.HasPrefix(manifest[0].URL, "https://github.com/XapaJIaMnu/translateLocally/") {
			t.Fatalf("manifest %s: %+v", goos, manifest)
		}
		if goos == "windows" && (manifest[0].ArchiveMember != "" || manifest[0].File != "translateLocally.exe") {
			t.Fatal("se altero la descarga de Windows")
		}
		if goos == "linux" && manifest[0].ArchiveMember != "usr/bin/translateLocally" {
			t.Fatal("no se extrae el ejecutable Linux")
		}
	}
	if len(depManifestFor("linux", "arm64")) != 0 {
		t.Fatal("no hay un binario Linux ARM64 en el manifest")
	}
}

func TestExtractDependencyUsesOnlyExpectedArchiveMember(t *testing.T) {
	for _, member := range []string{"./usr/bin/translateLocally", "../../escape"} {
		base := t.TempDir()
		source := filepath.Join(base, "source.deb")
		dest := filepath.Join(base, "engine")
		if err := os.WriteFile(source, makeDepDeb(t, member, []byte("engine bytes")), 0o600); err != nil {
			t.Fatal(err)
		}
		err := extractDepDeb(source, dest, "usr/bin/translateLocally")
		if strings.HasPrefix(member, "./usr/") {
			data, readErr := os.ReadFile(dest)
			if err != nil || readErr != nil || string(data) != "engine bytes" {
				t.Fatalf("extraccion: %v %v %q", err, readErr, data)
			}
		} else if err == nil {
			t.Fatal("se acepto un miembro ajeno")
		}
	}
}

func TestLinuxDependencyInstallIsVerifiedAtomicAndExecutable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("instalador Linux")
	}
	t.Setenv("NOUMON_LIBRARY_DATA", t.TempDir())
	for _, tc := range []struct {
		name, program string
		badHash       bool
		success       bool
	}{
		{"valid", "#!/bin/sh\nexit 0\n", false, true},
		{"bad hash", "#!/bin/sh\nexit 0\n", true, false},
		{"cannot run", "#!/bin/sh\nexit 5\n", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := makeDepDeb(t, "./usr/bin/translateLocally", []byte(tc.program))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
			defer server.Close()
			sum := sha256.Sum256(body)
			tool := depTool{File: "translateLocally", ID: "translateLocally", URL: server.URL, Bytes: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), ArchiveMember: "usr/bin/translateLocally"}
			if tc.badHash {
				tool.SHA256 = strings.Repeat("0", 64)
			}
			dest := depInstallPath(tool.File)
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dest, []byte("previous"), 0o755); err != nil {
				t.Fatal(err)
			}
			state := &depState{Installing: true}
			installDepTool(tool, state)
			data, err := os.ReadFile(dest)
			if err != nil {
				t.Fatal(err)
			}
			if tc.success {
				if state.Err != "" || string(data) != tc.program {
					t.Fatalf("instalacion: %q %q", state.Err, data)
				}
			} else if state.Err == "" || string(data) != "previous" {
				t.Fatalf("se reemplazo un ejecutable valido tras un fallo: %q %q", state.Err, data)
			}
			if state.Installing {
				t.Fatal("la instalacion no termino")
			}
			for _, suffix := range []string{".part", ".ready"} {
				if _, err := os.Stat(dest + suffix); !os.IsNotExist(err) {
					t.Fatalf("queda temporal %s", suffix)
				}
			}
		})
	}
}
