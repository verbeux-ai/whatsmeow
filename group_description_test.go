// Copyright (c) 2026 Verbeux AI
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

func TestBuildGroupDescriptionNodeUpdate(t *testing.T) {
	const description = "community description"
	const previousID = "previous-description-id"
	const newID types.MessageID = "3EB0000000000000000000000000000000000000"

	node := buildGroupDescriptionNode(description, previousID, newID)
	if node.Tag != "description" {
		t.Fatalf("unexpected tag: got %q, want %q", node.Tag, "description")
	}
	if node.Attrs["id"] != newID {
		t.Fatalf("unexpected id attribute: got %v, want %q", node.Attrs["id"], newID)
	}
	if node.Attrs["prev"] != previousID {
		t.Fatalf("unexpected prev attribute: got %v, want %q", node.Attrs["prev"], previousID)
	}
	if _, exists := node.Attrs["delete"]; exists {
		t.Fatal("update node must not contain the delete attribute")
	}
	children, ok := node.Content.([]waBinary.Node)
	if !ok || len(children) != 1 {
		t.Fatalf("unexpected description content: %#v", node.Content)
	}
	if children[0].Tag != "body" || !bytes.Equal(children[0].Content.([]byte), []byte(description)) {
		t.Fatalf("unexpected body node: %#v", children[0])
	}
}

func TestBuildGroupDescriptionNodeDelete(t *testing.T) {
	const previousID = "previous-description-id"

	node := buildGroupDescriptionNode("", previousID, "")
	if node.Attrs["delete"] != "true" {
		t.Fatalf("unexpected delete attribute: got %v, want %q", node.Attrs["delete"], "true")
	}
	if node.Attrs["prev"] != previousID {
		t.Fatalf("unexpected prev attribute: got %v, want %q", node.Attrs["prev"], previousID)
	}
	if _, exists := node.Attrs["id"]; exists {
		t.Fatal("delete node must not contain the id attribute")
	}
	if node.Content != nil {
		t.Fatalf("delete node must not contain a body: %#v", node.Content)
	}
}

func TestBuildGroupDescriptionNodeWithoutPreviousID(t *testing.T) {
	node := buildGroupDescriptionNode("first description", "", "3EB0000000000000000000000000000000000000")
	if _, exists := node.Attrs["prev"]; exists {
		t.Fatal("first description node must not contain the prev attribute")
	}
}

func TestGenerateCommunityDescriptionID(t *testing.T) {
	const randomHexLength = 36
	const iterations = 32

	seen := make(map[types.MessageID]struct{}, iterations)
	for range iterations {
		id := generateCommunityDescriptionID()
		if len(id) != len(WebMessageIDPrefix)+randomHexLength {
			t.Fatalf("unexpected community description ID length: got %d, want %d", len(id), len(WebMessageIDPrefix)+randomHexLength)
		}
		if !strings.HasPrefix(string(id), WebMessageIDPrefix) {
			t.Fatalf("community description ID %q is missing prefix %q", id, WebMessageIDPrefix)
		}
		randomPart := string(id[len(WebMessageIDPrefix):])
		if randomPart != strings.ToUpper(randomPart) {
			t.Fatalf("community description ID %q is not uppercase", id)
		}
		if _, err := hex.DecodeString(randomPart); err != nil {
			t.Fatalf("community description ID %q is not hexadecimal: %v", id, err)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate community description ID generated: %q", id)
		}
		seen[id] = struct{}{}
	}
}
