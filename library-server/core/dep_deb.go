package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// El paquete se verifica antes mediante SHA256. Solo extraemos el miembro
// previsto a un destino fijo: nunca ejecutamos scripts ni expandimos rutas.
func extractDepDeb(source, destination, member string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	magic := make([]byte, 8)
	if _, err := io.ReadFull(input, magic); err != nil || string(magic) != "!<arch>\n" {
		return fmt.Errorf("paquete de traduccion no valido")
	}
	for {
		header := make([]byte, 60)
		if _, err := io.ReadFull(input, header); err != nil {
			return fmt.Errorf("paquete sin datos de traduccion: %w", err)
		}
		if string(header[58:]) != "`\n" {
			return fmt.Errorf("cabecera de paquete no valida")
		}
		size, err := strconv.ParseInt(strings.TrimSpace(string(header[48:58])), 10, 64)
		if err != nil || size < 0 || size > 256<<20 {
			return fmt.Errorf("tamano de paquete no valido")
		}
		name := strings.TrimSuffix(strings.TrimSpace(string(header[:16])), "/")
		limited := io.LimitReader(input, size)
		if name == "data.tar.gz" {
			compressed, err := gzip.NewReader(limited)
			if err != nil {
				return err
			}
			defer compressed.Close()
			return extractDepTar(compressed, destination, member)
		}
		if _, err := io.Copy(io.Discard, limited); err != nil {
			return err
		}
		if size%2 != 0 {
			if _, err := io.CopyN(io.Discard, input, 1); err != nil {
				return err
			}
		}
	}
}

func extractDepTar(input io.Reader, destination, member string) error {
	archive := tar.NewReader(input)
	for {
		header, err := archive.Next()
		if err != nil {
			return fmt.Errorf("ejecutable no encontrado en el paquete: %w", err)
		}
		if strings.TrimPrefix(header.Name, "./") != member {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > 128<<20 {
			return fmt.Errorf("ejecutable de traduccion no valido")
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, archive)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
}

func checkDepExecutable(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, path, "-v")
	command.Env = append(os.Environ(), "QT_QPA_PLATFORM=offscreen")
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("no se pudo iniciar el motor de traduccion (%v): %.2000s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
