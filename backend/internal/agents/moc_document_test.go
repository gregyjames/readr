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
	if !strings.Contains(serialized, "[[Raft Consensus Algorithm]]") || !strings.Contains(serialized, "*User custom note here.*") {
		t.Errorf("roundtrip serialization missed content:\n%s", serialized)
	}
	if !strings.Contains(serialized, "## Executive Overview") || !strings.Contains(serialized, "High level synthesis of distributed systems.") {
		t.Errorf("roundtrip serialization missed overview:\n%s", serialized)
	}
}

func TestMOCDocument_ReconcileLinks_PrunesStaleBulletsOnly(t *testing.T) {
	raw := `# MOC - Test

### Core
- [[Active Note]] - Kept.
- [[Deleted Note]] - Should be pruned.

## Notes & Synthesis
*Keep this text.*
`
	doc, err := ParseMOCDocument(raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	pruned := doc.ReconcileLinks(map[string]bool{"Active Note": true, "active note": true})
	if !pruned {
		t.Errorf("expected pruned to be true")
	}

	out := doc.Serialize()
	if strings.Contains(out, "[[Deleted Note]]") {
		t.Errorf("expected Deleted Note to be pruned from output:\n%s", out)
	}
	if !strings.Contains(out, "[[Active Note]]") {
		t.Errorf("expected Active Note to remain in output:\n%s", out)
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
