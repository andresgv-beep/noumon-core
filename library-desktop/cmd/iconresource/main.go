package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/tc-hib/winres"
)

// Tamaños estándar de icono de aplicación en Windows (Explorer, taskbar, alt-tab).
var iconSizes = []int{256, 128, 64, 48, 32, 24, 16}

func main() {
	iconPath := flag.String("icon", "", "ICO or PNG file to embed")
	outputPath := flag.String("out", "", "COFF .syso output file")
	icoPath := flag.String("ico", "", "optional multi-size .ico output file")
	flag.Parse()
	if *iconPath == "" || (*outputPath == "" && *icoPath == "") {
		fmt.Fprintln(os.Stderr, "usage: iconresource -icon app.{ico|png} [-out app.syso] [-ico app.ico]")
		os.Exit(2)
	}

	iconFile, err := os.Open(*iconPath)
	if err != nil {
		fatal(err)
	}
	defer iconFile.Close()

	var icon *winres.Icon
	if strings.EqualFold(filepath.Ext(*iconPath), ".png") {
		img, err := png.Decode(iconFile)
		if err != nil {
			fatal(fmt.Errorf("decode png: %w", err))
		}
		icon, err = winres.NewIconFromResizedImage(img, iconSizes)
		if err != nil {
			fatal(fmt.Errorf("build icon: %w", err))
		}
	} else {
		icon, err = winres.LoadICO(iconFile)
		if err != nil {
			fatal(fmt.Errorf("load icon: %w", err))
		}
	}
	if *icoPath != "" {
		icoOut, err := os.Create(*icoPath)
		if err != nil {
			fatal(err)
		}
		if err := icon.SaveICO(icoOut); err != nil {
			icoOut.Close()
			fatal(fmt.Errorf("save ico: %w", err))
		}
		if err := icoOut.Close(); err != nil {
			fatal(err)
		}
	}

	if *outputPath == "" {
		return
	}
	resources := winres.ResourceSet{}
	if err := resources.SetIcon(winres.RT_ICON, icon); err != nil {
		fatal(fmt.Errorf("set icon: %w", err))
	}

	output, err := os.Create(*outputPath)
	if err != nil {
		fatal(err)
	}
	defer output.Close()
	if err := resources.WriteObject(output, winres.ArchAMD64); err != nil {
		fatal(fmt.Errorf("write resources: %w", err))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
