package httpserver

import (
	"strconv"
	"strings"
)

const (
	glipzAppVersion              = "0.0.2"
	federationProtocolName       = "glipz-federation"
	federationProtocolVersion    = federationProtocolName + "/3"
	federationEventSchemaVersion = 3
)

var federationSupportedProtocolVersions = []string{
	federationProtocolName + "/2",
	federationProtocolVersion,
}

func (s *Server) appVersion() string {
	if s != nil {
		if version := strings.TrimSpace(s.cfg.GlipzVersion); version != "" {
			return version
		}
	}
	return glipzAppVersion
}

func normalizeFederationEventVersion(v int) int {
	if v <= 0 {
		return 1
	}
	return v
}

func federationDiscoverySupportsCurrentProtocol(server federationServerDiscovery) bool {
	versions := make([]string, 0, len(server.SupportedProtocolVersions)+1)
	for _, version := range server.SupportedProtocolVersions {
		version = strings.TrimSpace(version)
		if version != "" {
			versions = append(versions, version)
		}
	}
	if version := strings.TrimSpace(server.ProtocolVersion); version != "" {
		versions = append(versions, version)
	}
	if len(versions) == 0 {
		return true
	}
	for _, version := range versions {
		name, major, ok := parseFederationProtocolVersion(version)
		if !ok || !strings.EqualFold(name, federationProtocolName) {
			continue
		}
		if major >= 2 {
			return true
		}
	}
	return false
}

func parseFederationProtocolVersion(raw string) (string, int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0, false
	}
	parts := strings.Split(raw, "/")
	if len(parts) != 2 {
		return "", 0, false
	}
	major, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return "", 0, false
	}
	return strings.TrimSpace(parts[0]), major, true
}

func (s *Server) patreonFeatureEnabled() bool {
	return s.cfg.PatreonEnabled
}

func (s *Server) membershipProviderEnabled(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "patreon":
		return s.patreonIntegrationAvailable()
	default:
		return false
	}
}
