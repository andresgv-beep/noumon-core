// mkdeb: genera un paquete .deb desde Windows sin dpkg.
//
// Un .deb es un archivo "ar" con tres miembros en orden: debian-binary,
// control.tar.gz y data.tar.gz. Aquí se construyen los tres con la stdlib.
//
// Uso:
//
//	mkdeb -staging <dir> -control <dir> -out paquete.deb
//
// -staging es la raíz del sistema de archivos a instalar (p. ej. contiene
// opt/noumon/... y lib/systemd/...). -control contiene control, postinst,
// prerm, etc. Los scripts de mantenimiento reciben modo 0755 automáticamente.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var maintainerScripts = map[string]bool{
	"postinst": true, "preinst": true, "postrm": true, "prerm": true, "config": true,
}

func main() {
	staging := flag.String("staging", "", "directorio raiz con el arbol a instalar")
	control := flag.String("control", "", "directorio con control, postinst, prerm...")
	out := flag.String("out", "", "ruta del .deb resultante")
	flag.Parse()
	if *staging == "" || *control == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: mkdeb -staging dir -control dir -out pkg.deb")
		os.Exit(2)
	}

	controlTar, err := tarDir(*control, "", func(name string) int64 {
		if maintainerScripts[name] {
			return 0o755
		}
		return 0o644
	})
	fatalIf("control.tar.gz", err)

	dataTar, err := tarDir(*staging, "", func(name string) int64 { return -1 })
	fatalIf("data.tar.gz", err)

	f, err := os.Create(*out)
	fatalIf("crear salida", err)
	defer f.Close()

	now := time.Now().Unix()
	_, err = f.WriteString("!<arch>\n")
	fatalIf("cabecera ar", err)
	fatalIf("debian-binary", arMember(f, "debian-binary", now, []byte("2.0\n")))
	fatalIf("control member", arMember(f, "control.tar.gz", now, controlTar))
	fatalIf("data member", arMember(f, "data.tar.gz", now, dataTar))

	info, _ := f.Stat()
	fmt.Printf("OK %s (%d bytes)\n", *out, info.Size())
}

// arMember escribe un miembro del archivo ar con relleno a tamaño par.
func arMember(w io.Writer, name string, mtime int64, data []byte) error {
	if _, err := fmt.Fprintf(w, "%-16s%-12d%-6d%-6d%-8s%-10d`\n", name, mtime, 0, 0, "100644", len(data)); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if len(data)%2 == 1 {
		if _, err := w.Write([]byte("\n")); err != nil {
			return err
		}
	}
	return nil
}

// tarDir empaqueta dir en un tar.gz con rutas "./..." y dueño root:root.
// modeFor devuelve el modo a forzar para un nombre de archivo (-1 = conservar
// el bit de ejecución si el nombre no tiene extensión, si no 0644).
func tarDir(dir, prefix string, modeFor func(name string) int64) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != dir {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	for _, path := range paths {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		hdr := &tar.Header{
			Name:    "./" + rel,
			ModTime: info.ModTime(),
			Uid:     0, Gid: 0,
			Uname: "root", Gname: "root",
		}
		if info.IsDir() {
			hdr.Typeflag = tar.TypeDir
			hdr.Name += "/"
			hdr.Mode = 0o755
			if err := tw.WriteHeader(hdr); err != nil {
				return nil, err
			}
			continue
		}
		hdr.Typeflag = tar.TypeReg
		hdr.Size = info.Size()
		mode := modeFor(filepath.Base(path))
		if mode < 0 {
			// Heurística Windows: los binarios van sin extensión; el resto son
			// recursos. Ejecutables 0755, resto 0644.
			if strings.Contains(filepath.Base(path), ".") {
				mode = 0o644
			} else {
				mode = 0o755
			}
		}
		hdr.Mode = mode
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		fh, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(tw, fh)
		fh.Close()
		if err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func fatalIf(step string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", step, err)
		os.Exit(1)
	}
}
