package whatsmeow

import (
	"strings"
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

func TestAppendCreateGroupSettingsCommunityUsesParentPayload(t *testing.T) {
	nodes := appendCreateGroupSettings(nil, ReqCreateGroup{
		Description: "community description",
		GroupParent: types.GroupParent{IsParent: true},
	})

	if len(nodes) != 2 {
		t.Fatalf("expected description and parent nodes, got %#v", nodes)
	}
	if nodes[0].Tag != "description" || nodes[1].Tag != "parent" {
		t.Fatalf("unexpected community create payload order: %#v", nodes)
	}
	if got := nodes[1].Attrs["default_membership_approval_mode"]; got != "request_required" {
		t.Fatalf("unexpected default membership approval mode: %v", got)
	}

	descriptionID, ok := nodes[0].Attrs["id"].(types.MessageID)
	if !ok || !strings.HasPrefix(string(descriptionID), WebMessageIDPrefix) {
		t.Fatalf("unexpected community description ID: %#v", nodes[0].Attrs["id"])
	}
	descriptionBody, ok := nodes[0].Content.([]waBinary.Node)
	if !ok || len(descriptionBody) != 1 || descriptionBody[0].Tag != "body" || string(descriptionBody[0].Content.([]byte)) != "community description" {
		t.Fatalf("unexpected inline community description: %#v", nodes[0].Content)
	}

	for _, node := range nodes {
		switch node.Tag {
		case "member_add_mode", "ephemeral", "membership_approval_mode":
			t.Fatalf("ordinary group setting %q must not be sent when creating a community parent", node.Tag)
		}
	}
}

func TestAppendCreateGroupSettingsCommunityWithoutDescription(t *testing.T) {
	nodes := appendCreateGroupSettings(nil, ReqCreateGroup{
		GroupParent: types.GroupParent{IsParent: true},
	})
	if len(nodes) != 1 || nodes[0].Tag != "parent" {
		t.Fatalf("unexpected community create payload: %#v", nodes)
	}
}

func TestAppendCreateGroupSettingsOrdinaryGroupKeepsDefaults(t *testing.T) {
	nodes := appendCreateGroupSettings(nil, ReqCreateGroup{})
	wantTags := []string{"member_add_mode", "ephemeral", "membership_approval_mode"}
	if len(nodes) != len(wantTags) {
		t.Fatalf("unexpected ordinary group create payload: %#v", nodes)
	}
	for index, want := range wantTags {
		if nodes[index].Tag != want {
			t.Fatalf("node %d: got %q, want %q", index, nodes[index].Tag, want)
		}
	}
	if got := nodes[0].Content; got != string(types.GroupMemberAddModeAllMember) {
		t.Fatalf("unexpected default member add mode: %v", got)
	}
}
