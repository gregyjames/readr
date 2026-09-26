package agents

import (
	"strings"
	"testing"
)

func TestMOCDocument_ParseAndRoundtrip(t *testing.T) {
	raw := `---
type: moc
title: MOC - Distributed Systems
tags:
  - moc
  - distributed-systems
---

# MOC - Distributed Systems

## Executive Overview
High level synthesis of distributed systems.

## Curated Index

### Consensus & Raft
- [[Raft Consensus Algorithm]] - Core consensus paper.
- [[Paxos Made Simple]] - Classical consensus.

### Storage Engines
- [[LSM Trees]] - Write-optimized storage.

## Notes & Synthesis
<!-- Content below this line is preserved across automated Librarian updates -->
*User custom note here.*
`

	doc, err := ParseMOCDocument(raw)
	if err != nil {
		t.Fatalf("unexpected error parsing MOC: %v", err)
	}

	if doc.Title != "MOC - Distributed Systems" {
		t.Errorf("expected title 'MOC - Distributed Systems', got %q", doc.Title)
	}
	if !doc.HasCustomUserNotes() {
		t.Errorf("expected custom user notes to be detected")
	}
	if len(doc.CuratedSections) != 2 {
		t.Fatalf("expected 2 curated sections, got %d", len(doc.CuratedSections))
	}
	if doc.CuratedSections[0].Title != "Consensus & Raft" {
		t.Errorf("expected section 0 title 'Consensus & Raft', got %q", doc.CuratedSections[0].Title)
	}
	if len(doc.CuratedSections[0].Items) != 2 {
		t.Errorf("expected 2 items in section 0, got %d", len(doc.CuratedSections[0].Items))
	}

	serialized := doc.Serialize()
	if !strings.Contains(serialized, "type: moc") || !strings.Contains(serialized, "title: MOC - Distributed Systems") {
		t.Errorf("roundtrip serialization missed frontmatter:\n%s", serialized)
	}
	if !strings.Contains(serialized, "[[Raft Consensus Algorithm]]") || !strings.Contains(serialized, "*User custom note here.*") {
		t.Errorf("roundtrip serialization missed content:\n%s", serialized)
	}
	if !strings.Contains(serialized, "## Executive Overview") || !strings.Contains(serialized, "High level synthesis of distributed systems.") {
		t.Errorf("roundtrip serialization missed overview:\n%s", serialized)
	}
}

func TestMOCDocument_ParseAndRoundtrip_PreservesRawYAMLWhenNonMap(t *testing.T) {
	raw := `---
some unparseable or scalar yaml
---

# MOC - Unstructured

## Notes & Synthesis
<!-- Content below this line is preserved across automated Librarian updates -->
`
	doc, err := ParseMOCDocument(raw)
	if err != nil {
		t.Fatalf("unexpected error parsing MOC: %v", err)
	}

	serialized := doc.Serialize()
	if !strings.Contains(serialized, "some unparseable or scalar yaml") {
		t.Errorf("expected raw YAML to be preserved in serialization when Frontmatter is empty/non-map, got:\n%s", serialized)
	}
}

func TestMOCDocument_ReconcileLinks_PrunesStaleBulletsOnly(t *testing.T) {
	raw := `# MOC - Test

### Core
- [[Active Note]] - Kept.
- [[Deleted Note]] - Should be pruned.

### Empty Topic
- [[Obsolete Note]] - Pruned.

### Plain Title Section
- Plain Dead Note - Describes a deleted note.
- [[Active Note 2]] - Preserved.

## Notes & Synthesis
*Keep this text.*
`
	doc, err := ParseMOCDocument(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	pruned := doc.ReconcileLinks(map[string]bool{
		"Active Note":   true,
		"active note":   true,
		"Active Note 2": true,
		"active note 2": true,
	})
	if !pruned {
		t.Errorf("expected pruned to be true")
	}

	out := doc.Serialize()
	if strings.Contains(out, "[[Deleted Note]]") {
		t.Errorf("expected Deleted Note to be pruned from output:\n%s", out)
	}
	if strings.Contains(out, "### Empty Topic") {
		t.Errorf("expected Empty Topic section heading to be pruned when all items are deleted:\n%s", out)
	}
	if strings.Contains(out, "Plain Dead Note") {
		t.Errorf("expected Plain Dead Note to be pruned from output:\n%s", out)
	}
	if !strings.Contains(out, "[[Active Note]]") || !strings.Contains(out, "[[Active Note 2]]") {
		t.Errorf("expected Active Notes to remain in output:\n%s", out)
	}
	if !strings.Contains(out, "*Keep this text.*") {
		t.Errorf("expected user notes to be preserved:\n%s", out)
	}
}

func TestMOCDocument_ApplyDeltaPlacements_AppendsAndDeduplicates(t *testing.T) {
	raw := `# MOC - Distributed Systems

## Curated Index

### Core Concepts
- [[Raft]] - Consensus.

## Notes & Synthesis
*User synthesis.*
`
	doc, err := ParseMOCDocument(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	placements := []MOCDeltaPlacement{
		{ArticleID: 2, TargetSection: "Core Concepts", ContextNote: "New note 2"},
		{ArticleID: 2, TargetSection: "Core Concepts", ContextNote: "Duplicate note 2"},
		{ArticleID: 3, TargetSection: "Advanced Topics", ContextNote: "New note 3"},
	}

	articleMap := map[int64]MOCArticleInfo{
		2: {ID: 2, Title: "Paxos", FilePath: "/articles/Distributed Systems/Paxos.md"},
		3: {ID: 3, Title: "Byzantine Agreement", FilePath: "/articles/Distributed Systems/Byzantine Agreement.md"},
	}

	doc.ApplyDeltaPlacements(placements, articleMap)
	out := doc.Serialize()

	// Paxos should appear exactly once
	if strings.Count(out, "[[Paxos]]") != 1 {
		t.Errorf("expected [[Paxos]] to appear exactly once, got count %d in:\n%s", strings.Count(out, "[[Paxos]]"), out)
	}

	// Byzantine Agreement should appear in Advanced Topics
	if !strings.Contains(out, "### Advanced Topics") || !strings.Contains(out, "[[Byzantine Agreement]]") {
		t.Errorf("expected Advanced Topics with Byzantine Agreement in:\n%s", out)
	}

	// User notes preserved
	if !strings.Contains(out, "*User synthesis.*") {
		t.Errorf("expected user synthesis preserved in:\n%s", out)
	}
}

func TestMOCDocument_PreservesCustomFrontmatterAndUserNotes(t *testing.T) {
	raw := `---
type: moc
title: MOC - Distributed Systems
tags:
  - moc
  - distributed-systems
custom_prop: custom_val
aliases:
  - DistSys
  - DS Hub
created: 2026-01-01
---

# MOC - Distributed Systems

## Executive Overview
High level overview.

## Curated Index

### Section 1
- [[Node 1]] - Note 1

## Personal Notes & Thoughts
### Deep Dive
Here is my deep dive analysis.
- Important thought 1
- Important thought 2
`

	doc, err := ParseMOCDocument(raw)
	if err != nil {
		t.Fatalf("unexpected error parsing MOC: %v", err)
	}

	// Verify frontmatter keys preserved
	if doc.Frontmatter["custom_prop"] != "custom_val" {
		t.Errorf("expected custom_prop 'custom_val', got %v", doc.Frontmatter["custom_prop"])
	}
	aliases, ok := doc.Frontmatter["aliases"].([]interface{})
	if !ok || len(aliases) != 2 || aliases[0] != "DistSys" || aliases[1] != "DS Hub" {
		t.Errorf("expected aliases slice ['DistSys', 'DS Hub'], got %v", doc.Frontmatter["aliases"])
	}
	if doc.Frontmatter["created"] == nil {
		t.Errorf("expected created key to be present")
	}

	// Verify user notes preserved
	if !doc.HasCustomUserNotes() {
		t.Errorf("expected HasCustomUserNotes to be true")
	}
	if !strings.Contains(doc.UserNotesBody, "Here is my deep dive analysis.") {
		t.Errorf("expected user notes to contain analysis, got: %q", doc.UserNotesBody)
	}
	if !strings.Contains(doc.UserNotesBody, "## Personal Notes & Thoughts") {
		t.Errorf("expected user notes to preserve custom header '## Personal Notes & Thoughts', got: %q", doc.UserNotesBody)
	}

	serialized := doc.Serialize()
	if !strings.Contains(serialized, "custom_prop: custom_val") {
		t.Errorf("expected serialized to contain custom_prop: custom_val, got:\n%s", serialized)
	}
	if !strings.Contains(serialized, "DistSys") || !strings.Contains(serialized, "DS Hub") {
		t.Errorf("expected serialized to contain aliases, got:\n%s", serialized)
	}
	if !strings.Contains(serialized, "## Personal Notes & Thoughts") {
		t.Errorf("expected serialized to contain user heading '## Personal Notes & Thoughts', got:\n%s", serialized)
	}
	if !strings.Contains(serialized, "Here is my deep dive analysis.") {
		t.Errorf("expected serialized to contain custom notes body, got:\n%s", serialized)
	}
	// Make sure duplicate '## Notes & Synthesis' wasn't incorrectly injected if user had custom header
	if strings.Contains(serialized, "## Notes & Synthesis") {
		t.Errorf("expected serialized not to force '## Notes & Synthesis' when custom user notes header was used, got:\n%s", serialized)
	}
}

func TestAssembleMOCMarkdown_PreservesExistingCustomFrontmatterAndNotes(t *testing.T) {
	existing := `---
type: moc
title: MOC - Distributed Systems
tags:
  - moc
  - distributed-systems
custom_prop: custom_val
aliases:
  - DistSys
---

# MOC - Distributed Systems

## Executive Overview
Old overview.

## Curated Index

### Section 1
- [[Node 1]] - Note 1

## Personal Notes & Thoughts
### Deep Dive
Keep this analysis.
`

	synthesis := &MOCSynthesisResponse{
		TopicTitle:       "Distributed Systems",
		ExecutiveSummary: "Brand new automated overview.",
		Sections: []MOCSection{
			{
				Title: "Section 1",
				Items: []MOCItem{
					{ArticleID: 10, ContextNote: "Updated note 10"},
				},
			},
		},
	}

	articleMap := map[int64]MOCArticleInfo{
		10: {ID: 10, Title: "Raft Consensus", FilePath: "/vault/Raft.md"},
	}

	assembled := assembleMOCMarkdown(synthesis, "MOC - Distributed Systems", "distributed-systems", existing, articleMap)

	if !strings.Contains(assembled, "custom_prop: custom_val") {
		t.Errorf("expected custom_prop to be preserved, got:\n%s", assembled)
	}
	if !strings.Contains(assembled, "DistSys") {
		t.Errorf("expected aliases to be preserved, got:\n%s", assembled)
	}
	if !strings.Contains(assembled, "## Personal Notes & Thoughts") || !strings.Contains(assembled, "Keep this analysis.") {
		t.Errorf("expected personal notes to be preserved, got:\n%s", assembled)
	}
	if strings.Contains(assembled, "## Notes & Synthesis") {
		t.Errorf("expected not to inject duplicate ## Notes & Synthesis when custom header present, got:\n%s", assembled)
	}
}
