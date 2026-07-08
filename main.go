package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmcateer/switch-tester-5000/internal/config"
	"github.com/jmcateer/switch-tester-5000/internal/tui"
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		exe = os.Args[0]
	}
	exe, _ = filepath.EvalSymlinks(exe)
	dir := filepath.Dir(exe)
	base := filepath.Base(exe)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	cfgPath := filepath.Join(dir, base+".toml")

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	app := tui.New(cfg, cfgPath)
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
