package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The game's ProjectZomboid64 launcher reads this json and passes vmArgs on
// the java command line. Command-line options override _JAVA_OPTIONS, so
// without patching this file MAX_RAM/MIN_RAM/GC_CONFIG would be silently
// ignored and the game's hardcoded -Xmx8g/-XX:+UseZGC would win.
type launcherJson struct {
	MainClass string   `json:"mainClass"`
	Classpath []string `json:"classpath"`
	VMArgs    []string `json:"vmArgs"`
}

// PatchLauncherJson rewrites <ServerDir>/ProjectZomboid64.json so the JVM
// heap and GC settings from the environment take effect. It is idempotent
// and preserves every vmArg it does not own. A missing file is not an error
// (the _JAVA_OPTIONS fallback in the process setup still applies).
func (c *ServerConfig) PatchLauncherJson() error {
	path := c.ServerDir + "/ProjectZomboid64.json"
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("WARNING: %s not found, relying on _JAVA_OPTIONS for JVM settings\n", path)
			return nil
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}

	var launcher launcherJson
	if err := json.Unmarshal(data, &launcher); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	kept := make([]string, 0, len(launcher.VMArgs)+4)
	for _, arg := range launcher.VMArgs {
		switch {
		case strings.HasPrefix(arg, "-Xmx") || strings.HasPrefix(arg, "-Xms"):
			// replaced below
		case strings.HasPrefix(arg, "-XX:+Use") && strings.HasSuffix(arg, "GC"):
			// replaced below from GC_CONFIG
		default:
			kept = append(kept, arg)
		}
	}

	vmArgs := []string{
		fmt.Sprintf("-Xms%s", c.MinRam),
		fmt.Sprintf("-Xmx%s", c.MaxRam),
	}
	gc := c.GCConfig
	if gc != "" {
		if !strings.HasSuffix(gc, "GC") {
			gc += "GC"
		}
		vmArgs = append(vmArgs, fmt.Sprintf("-XX:+Use%s", gc))
	}
	vmArgs = append(vmArgs, kept...)
	if c.JvmExtraArgs != "" {
		vmArgs = append(vmArgs, strings.Fields(c.JvmExtraArgs)...)
	}
	launcher.VMArgs = vmArgs

	out, err := json.MarshalIndent(launcher, "", "\t")
	if err != nil {
		return fmt.Errorf("serializing launcher config: %w", err)
	}
	out = append(out, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating launcher config directory: %w", err)
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
