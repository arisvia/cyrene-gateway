package mitm

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// DNSManager handles /etc/hosts manipulation for MITM domain interception.
type DNSManager struct {
	mu        sync.Mutex
	hostsFile string
}

func NewDNSManager() *DNSManager {
	hostsFile := "/etc/hosts"
	if runtime.GOOS == "windows" {
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			systemRoot = `C:\Windows`
		}
		hostsFile = systemRoot + `\System32\drivers\etc\hosts`
	}
	return &DNSManager{hostsFile: hostsFile}
}

// CheckStatus returns per-tool DNS active status.
func (dm *DNSManager) CheckStatus() map[string]bool {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	content, err := os.ReadFile(dm.hostsFile)
	if err != nil {
		result := make(map[string]bool)
		for tool := range ToolHosts {
			result[tool] = false
		}
		return result
	}

	text := string(content)
	result := make(map[string]bool)
	for tool, hosts := range ToolHosts {
		allPresent := true
		for _, h := range hosts {
			if !strings.Contains(text, h) {
				allPresent = false
				break
			}
		}
		result[tool] = allPresent
	}
	return result
}

// AddEntry adds DNS entries for a tool (requires root/admin).
func (dm *DNSManager) AddEntry(tool string) error {
	hosts, ok := ToolHosts[tool]
	if !ok {
		return fmt.Errorf("unknown tool: %s", tool)
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()

	content, err := os.ReadFile(dm.hostsFile)
	if err != nil {
		return fmt.Errorf("cannot read hosts file: %w", err)
	}

	text := string(content)
	var toAdd []string
	for _, h := range hosts {
		if !strings.Contains(text, h) {
			toAdd = append(toAdd, h)
		}
	}
	if len(toAdd) == 0 {
		return nil
	}

	// Append entries
	var sb strings.Builder
	sb.WriteString(strings.TrimRight(text, "\n\r "))
	for _, h := range toAdd {
		sb.WriteString("\n127.0.0.1 " + h)
	}
	sb.WriteString("\n")

	if err := os.WriteFile(dm.hostsFile, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("cannot write hosts file (need root): %w", err)
	}

	dm.flushDNS()
	return nil
}

// RemoveEntry removes DNS entries for a tool.
func (dm *DNSManager) RemoveEntry(tool string) error {
	hosts, ok := ToolHosts[tool]
	if !ok {
		return fmt.Errorf("unknown tool: %s", tool)
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()

	return dm.removeHosts(hosts)
}

// RemoveAll removes all MITM DNS entries.
func (dm *DNSManager) RemoveAll() error {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	return dm.removeHosts(AllToolHosts())
}

func (dm *DNSManager) removeHosts(hosts []string) error {
	f, err := os.Open(dm.hostsFile)
	if err != nil {
		return err
	}

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		skip := false
		for _, h := range hosts {
			if strings.Contains(line, h) {
				skip = true
				break
			}
		}
		if !skip {
			lines = append(lines, line)
		}
	}
	f.Close()

	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(dm.hostsFile, []byte(content), 0o644); err != nil {
		return fmt.Errorf("cannot write hosts file (need root): %w", err)
	}

	dm.flushDNS()
	return nil
}

func (dm *DNSManager) flushDNS() {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("dscacheutil", "-flushcache").Run()
		exec.Command("killall", "-HUP", "mDNSResponder").Run()
	case "linux":
		exec.Command("resolvectl", "flush-caches").Run()
	case "windows":
		exec.Command("ipconfig", "/flushdns").Run()
	}
}
