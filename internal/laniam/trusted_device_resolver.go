package laniam

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

type DeviceResolver interface {
	ResolveCandidates(clientIP string) ([]models.TrustedDeviceCandidate, error)
	ResolveClientMAC(clientIP string) (models.TrustedDeviceCandidate, bool)
}

type NeighbourResolver struct{}

func NewNeighbourResolver() *NeighbourResolver { return &NeighbourResolver{} }

func (r *NeighbourResolver) ResolveCandidates(clientIP string) ([]models.TrustedDeviceCandidate, error) {
	if clientIP != "" {
		probeNeighbour(clientIP)
	}
	entries := map[string]models.TrustedDeviceCandidate{}
	mergeCandidateMap(entries, parseProcNetARP())
	mergeCandidateMap(entries, parseIPNeigh(clientIP))
	mergeCandidateMap(entries, parseIPNeigh(""))
	mergeCandidateMap(entries, parseARPCommand(clientIP))

	candidates := make([]models.TrustedDeviceCandidate, 0, len(entries))
	for _, candidate := range entries {
		if candidate.MacAddress == "" {
			continue
		}
		candidates = append(candidates, candidate)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		leftCurrent := clientIP != "" && candidates[i].SourceIP == clientIP
		rightCurrent := clientIP != "" && candidates[j].SourceIP == clientIP
		if leftCurrent != rightCurrent {
			return leftCurrent
		}
		leftTime := time.Time{}
		rightTime := time.Time{}
		if candidates[i].LastSeenAt != nil {
			leftTime = *candidates[i].LastSeenAt
		}
		if candidates[j].LastSeenAt != nil {
			rightTime = *candidates[j].LastSeenAt
		}
		if !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		return candidates[i].MacAddress < candidates[j].MacAddress
	})

	if len(candidates) == 0 {
		return nil, nil
	}
	return candidates, nil
}

// ResolveClientMAC returns the MAC observed for a single client IP (after ARP probe).
func (r *NeighbourResolver) ResolveClientMAC(clientIP string) (models.TrustedDeviceCandidate, bool) {
	if clientIP == "" || net.ParseIP(clientIP) == nil {
		return models.TrustedDeviceCandidate{}, false
	}
	if ip := net.ParseIP(clientIP); ip != nil && ip.To4() == nil {
		return models.TrustedDeviceCandidate{}, false
	}
	probeNeighbour(clientIP)
	entries := map[string]models.TrustedDeviceCandidate{}
	mergeCandidateMap(entries, parseIPNeigh(clientIP))
	mergeCandidateMap(entries, parseARPCommand(clientIP))
	for _, candidate := range entries {
		if candidate.SourceIP == clientIP {
			normalized, ok := NormalizeCandidate(candidate)
			if ok {
				return normalized, true
			}
		}
	}
	for _, candidate := range entries {
		normalized, ok := NormalizeCandidate(candidate)
		if ok {
			return normalized, true
		}
	}
	return models.TrustedDeviceCandidate{}, false
}

func ClientIPFromRequest(r *http.Request) string {
	for _, key := range []string{"X-Forwarded-For", "X-Real-IP"} {
		if values := r.Header.Values(key); len(values) > 0 {
			parts := strings.Split(values[0], ",")
			for _, part := range parts {
				ip := strings.TrimSpace(part)
				if parsed := net.ParseIP(ip); parsed != nil {
					return parsed.String()
				}
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if parsed := net.ParseIP(host); parsed != nil {
			return parsed.String()
		}
	}
	if parsed := net.ParseIP(r.RemoteAddr); parsed != nil {
		return parsed.String()
	}
	return ""
}

func NormalizeCandidate(input models.TrustedDeviceCandidate) (models.TrustedDeviceCandidate, bool) {
	mac := normalizeMAC(input.MacAddress)
	if mac == "" {
		return models.TrustedDeviceCandidate{}, false
	}
	candidate := input
	candidate.MacAddress = mac
	candidate.DeviceLabel = strings.TrimSpace(candidate.DeviceLabel)
	candidate.SourceIP = strings.TrimSpace(candidate.SourceIP)
	if candidate.DeviceLabel == "" {
		candidate.DeviceLabel = candidate.SourceIP
	}
	if candidate.DeviceLabel == "" {
		candidate.DeviceLabel = mac
	}
	return candidate, true
}

func normalizeMAC(input string) string {
	mac := strings.TrimSpace(strings.ToUpper(input))
	if mac == "" {
		return ""
	}
	parsed, err := net.ParseMAC(mac)
	if err != nil {
		return ""
	}
	normalized := strings.ToUpper(parsed.String())
	if !isUsableMAC(normalized) {
		return ""
	}
	return normalized
}

func isUsableMAC(mac string) bool {
	if mac == "" || mac == "00:00:00:00:00:00" {
		return false
	}
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return false
	}
	first, err := strconv.ParseUint(parts[0], 16, 8)
	if err != nil {
		return false
	}
	// Ignore multicast / locally administered noise from incomplete ARP rows.
	return first&0x01 == 0
}

func probeNeighbour(clientIP string) {
	if net.ParseIP(clientIP) == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", clientIP).Run()
}

func mergeCandidateMap(dst map[string]models.TrustedDeviceCandidate, inputs []models.TrustedDeviceCandidate) {
	for _, input := range inputs {
		candidate, ok := NormalizeCandidate(input)
		if !ok {
			continue
		}
		existing, exists := dst[candidate.MacAddress]
		if !exists {
			dst[candidate.MacAddress] = candidate
			continue
		}
		if existing.DeviceLabel == "" || existing.DeviceLabel == existing.SourceIP || existing.DeviceLabel == existing.MacAddress {
			existing.DeviceLabel = candidate.DeviceLabel
		}
		if existing.SourceIP == "" {
			existing.SourceIP = candidate.SourceIP
		}
		if existing.LastSeenAt == nil && candidate.LastSeenAt != nil {
			existing.LastSeenAt = candidate.LastSeenAt
		}
		dst[candidate.MacAddress] = existing
	}
}

func parseProcNetARP() []models.TrustedDeviceCandidate {
	file, err := os.Open("/proc/net/arp")
	if err != nil {
		return nil
	}
	defer file.Close()

	now := time.Now().UTC()
	var out []models.TrustedDeviceCandidate
	scanner := bufio.NewScanner(file)
	first := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		seen := now
		out = append(out, models.TrustedDeviceCandidate{
			SourceIP:    fields[0],
			MacAddress:  fields[3],
			DeviceLabel: fields[0],
			LastSeenAt:  &seen,
		})
	}
	return out
}

func parseIPNeigh(clientIP string) []models.TrustedDeviceCandidate {
	args := []string{"neigh", "show"}
	if clientIP != "" {
		args = append(args, clientIP)
	}
	cmd := exec.Command("ip", args...)
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return nil
	}
	return parseIPNeighOutput(output)
}

func parseIPNeighOutput(output []byte) []models.TrustedDeviceCandidate {
	now := time.Now().UTC()
	var out []models.TrustedDeviceCandidate
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 {
			continue
		}
		if fields[1] != "dev" {
			continue
		}
		ip := fields[0]
		mac := ""
		for idx := 0; idx < len(fields)-1; idx++ {
			if fields[idx] == "lladdr" {
				mac = fields[idx+1]
				break
			}
		}
		if mac == "" {
			continue
		}
		seen := now
		out = append(out, models.TrustedDeviceCandidate{SourceIP: ip, MacAddress: mac, DeviceLabel: ip, LastSeenAt: &seen})
	}
	return out
}

func parseARPCommand(clientIP string) []models.TrustedDeviceCandidate {
	if clientIP == "" {
		return nil
	}
	cmd := exec.Command("arp", "-an", clientIP)
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return nil
	}
	line := strings.TrimSpace(string(output))
	open := strings.Index(line, "(")
	close := strings.Index(line, ")")
	at := strings.Index(strings.ToLower(line), " at ")
	if open == -1 || close == -1 || at == -1 || at+4 >= len(line) {
		return nil
	}
	ip := strings.TrimSpace(line[open+1 : close])
	parts := strings.Fields(line[at+4:])
	if len(parts) == 0 {
		return nil
	}
	seen := time.Now().UTC()
	return []models.TrustedDeviceCandidate{{SourceIP: ip, MacAddress: parts[0], DeviceLabel: ip, LastSeenAt: &seen}}
}

func DebugCandidatesString(candidates []models.TrustedDeviceCandidate) string {
	parts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		parts = append(parts, fmt.Sprintf("%s(%s)", candidate.DeviceLabel, candidate.MacAddress))
	}
	return strings.Join(parts, ", ")
}
