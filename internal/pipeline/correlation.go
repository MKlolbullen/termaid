package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// CorrelationKey groups tool observations that describe the same underlying
// issue. Structured metadata wins; raw-value hashing is a safe fallback for
// legacy/unstructured tool output.
func CorrelationKey(record DataRecord) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(record.Metadata["asset"])),
		strings.ToLower(strings.TrimSpace(record.Metadata["location"])),
		strings.ToLower(strings.TrimSpace(record.Metadata["weakness"])),
		strings.ToLower(strings.TrimSpace(record.Metadata["evidence"])),
	}
	structured := false
	for _, p := range parts {
		if p != "" {
			structured = true
			break
		}
	}
	if !structured {
		parts = []string{strings.ToLower(strings.TrimSpace(record.Value))}
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:16])
}

// CorrelateRecords de-duplicates observations while retaining provenance from
// all contributing tools in metadata.sources. The highest-confidence record is
// used as the representative observation.
func CorrelateRecords(records []DataRecord) []DataRecord {
	type group struct {
		best    DataRecord
		sources map[string]struct{}
	}
	groups := make(map[string]*group)
	for _, record := range records {
		key := CorrelationKey(record)
		g, ok := groups[key]
		if !ok {
			copyRecord := record
			if copyRecord.Metadata == nil {
				copyRecord.Metadata = make(map[string]string)
			}
			g = &group{best: copyRecord, sources: make(map[string]struct{})}
			groups[key] = g
		}
		if record.Source != "" {
			g.sources[record.Source] = struct{}{}
		}
		if record.Confidence > g.best.Confidence {
			meta := g.best.Metadata
			g.best = record
			if g.best.Metadata == nil {
				g.best.Metadata = meta
			}
		}
	}

	out := make([]DataRecord, 0, len(groups))
	for key, g := range groups {
		if g.best.Metadata == nil {
			g.best.Metadata = make(map[string]string)
		}
		var sources []string
		for source := range g.sources {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		g.best.Metadata["correlation_key"] = key
		g.best.Metadata["sources"] = strings.Join(sources, ",")
		out = append(out, g.best)
	}
	sort.Slice(out, func(i, j int) bool {
		return CorrelationKey(out[i]) < CorrelationKey(out[j])
	})
	return out
}
