// Copyright (c) 2026 Verbeux AI
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

type groupInfoResult struct {
	info *types.GroupInfo
	err  error
}

type sentGroupIQ struct {
	iqType  infoQueryType
	jid     types.JID
	content waBinary.Node
}

type fakeGroupDescriptionClient struct {
	groupInfos   map[types.JID][]groupInfoResult
	generatedIDs []types.MessageID
	infoRequests []types.JID
	sends        []sentGroupIQ
	sendErrors   []error
}

func (fake *fakeGroupDescriptionClient) getGroupInfo(_ context.Context, jid types.JID, _ bool) (*types.GroupInfo, error) {
	fake.infoRequests = append(fake.infoRequests, jid)
	results := fake.groupInfos[jid]
	if len(results) == 0 {
		return nil, fmt.Errorf("unexpected group info request for %s", jid)
	}
	result := results[0]
	fake.groupInfos[jid] = results[1:]
	return result.info, result.err
}

func (fake *fakeGroupDescriptionClient) sendGroupIQ(_ context.Context, iqType infoQueryType, jid types.JID, content waBinary.Node) (*waBinary.Node, error) {
	fake.sends = append(fake.sends, sentGroupIQ{iqType: iqType, jid: jid, content: content})
	if len(fake.sendErrors) == 0 {
		return &waBinary.Node{}, nil
	}
	err := fake.sendErrors[0]
	fake.sendErrors = fake.sendErrors[1:]
	return nil, err
}

func (fake *fakeGroupDescriptionClient) GenerateMessageID() types.MessageID {
	if len(fake.generatedIDs) == 0 {
		return "generated-message-id"
	}
	id := fake.generatedIDs[0]
	fake.generatedIDs = fake.generatedIDs[1:]
	return id
}

func testGroupInfo(jid types.JID, topic, topicID string) *types.GroupInfo {
	info := &types.GroupInfo{JID: jid}
	info.Topic = topic
	info.TopicID = topicID
	return info
}

func requireDescriptionBody(t *testing.T, node waBinary.Node, expected string) {
	t.Helper()
	children, ok := node.Content.([]waBinary.Node)
	if !ok || len(children) != 1 || children[0].Tag != "body" {
		t.Fatalf("unexpected description body: %#v", node.Content)
	}
	body, ok := children[0].Content.([]byte)
	if !ok || string(body) != expected {
		t.Fatalf("unexpected description text: got %#v, want %q", children[0].Content, expected)
	}
}

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

func TestSetGroupDescriptionCommunityAnnouncementUsesParent(t *testing.T) {
	announcementJID := types.NewJID("announcement", types.GroupServer)
	parentJID := types.NewJID("parent", types.GroupServer)
	announcementInfo := testGroupInfo(announcementJID, "announcement description", "announcement-prev")
	announcementInfo.IsDefaultSubGroup = true
	announcementInfo.LinkedParentJID = parentJID
	parentInfo := testGroupInfo(parentJID, "old community description", "parent-prev")
	parentInfo.IsParent = true
	fake := &fakeGroupDescriptionClient{groupInfos: map[types.JID][]groupInfoResult{
		announcementJID: {{info: announcementInfo}},
		parentJID:       {{info: parentInfo}},
	}}

	err := setGroupDescription(context.Background(), fake, announcementJID, "new community description")
	if err != nil {
		t.Fatalf("SetGroupDescription returned an error: %v", err)
	}
	if len(fake.infoRequests) != 2 || fake.infoRequests[0] != announcementJID || fake.infoRequests[1] != parentJID {
		t.Fatalf("unexpected metadata requests: %#v", fake.infoRequests)
	}
	if len(fake.sends) != 1 {
		t.Fatalf("unexpected send count: got %d, want 1", len(fake.sends))
	}
	sent := fake.sends[0]
	if sent.iqType != iqSet || sent.jid != parentJID {
		t.Fatalf("description sent to wrong target: type=%q jid=%s", sent.iqType, sent.jid)
	}
	if sent.content.Attrs["prev"] != "parent-prev" {
		t.Fatalf("description used wrong previous ID: %#v", sent.content.Attrs)
	}
	id, ok := sent.content.Attrs["id"].(types.MessageID)
	if !ok || len(id) != 40 || !strings.HasPrefix(string(id), WebMessageIDPrefix) {
		t.Fatalf("description used invalid community ID: %#v", sent.content.Attrs["id"])
	}
	requireDescriptionBody(t, sent.content, "new community description")
}

func TestSetGroupDescriptionCommunityAnnouncementDeleteUsesParent(t *testing.T) {
	announcementJID := types.NewJID("announcement", types.GroupServer)
	parentJID := types.NewJID("parent", types.GroupServer)
	announcementInfo := testGroupInfo(announcementJID, "announcement description", "announcement-prev")
	announcementInfo.IsDefaultSubGroup = true
	announcementInfo.LinkedParentJID = parentJID
	parentInfo := testGroupInfo(parentJID, "community description", "parent-prev")
	parentInfo.IsParent = true
	fake := &fakeGroupDescriptionClient{groupInfos: map[types.JID][]groupInfoResult{
		announcementJID: {{info: announcementInfo}},
		parentJID:       {{info: parentInfo}},
	}}

	if err := setGroupDescription(context.Background(), fake, announcementJID, ""); err != nil {
		t.Fatalf("SetGroupDescription returned an error: %v", err)
	}
	if len(fake.sends) != 1 || fake.sends[0].jid != parentJID {
		t.Fatalf("delete sent to wrong target: %#v", fake.sends)
	}
	node := fake.sends[0].content
	if node.Attrs["delete"] != "true" || node.Attrs["prev"] != "parent-prev" {
		t.Fatalf("unexpected delete attributes: %#v", node.Attrs)
	}
	if _, ok := node.Attrs["id"]; ok || node.Content != nil {
		t.Fatalf("delete must not contain id or body: %#v", node)
	}
}

func TestSetGroupDescriptionOrdinarySubGroupStaysOnChild(t *testing.T) {
	childJID := types.NewJID("child", types.GroupServer)
	parentJID := types.NewJID("parent", types.GroupServer)
	childInfo := testGroupInfo(childJID, "old child description", "child-prev")
	childInfo.LinkedParentJID = parentJID
	fake := &fakeGroupDescriptionClient{
		groupInfos:   map[types.JID][]groupInfoResult{childJID: {{info: childInfo}}},
		generatedIDs: []types.MessageID{"normal-group-id"},
	}

	if err := setGroupDescription(context.Background(), fake, childJID, "new child description"); err != nil {
		t.Fatalf("SetGroupDescription returned an error: %v", err)
	}
	if len(fake.infoRequests) != 1 || fake.infoRequests[0] != childJID {
		t.Fatalf("ordinary subgroup unexpectedly resolved its parent: %#v", fake.infoRequests)
	}
	if len(fake.sends) != 1 || fake.sends[0].jid != childJID {
		t.Fatalf("ordinary subgroup description sent to wrong target: %#v", fake.sends)
	}
	if fake.sends[0].content.Attrs["id"] != types.MessageID("normal-group-id") {
		t.Fatalf("ordinary subgroup used wrong description ID: %#v", fake.sends[0].content.Attrs)
	}
}

func TestSetGroupDescriptionAnnouncementFailsClosedWithoutValidParent(t *testing.T) {
	announcementJID := types.NewJID("announcement", types.GroupServer)
	parentJID := types.NewJID("parent", types.GroupServer)

	t.Run("missing linked parent", func(t *testing.T) {
		announcementInfo := testGroupInfo(announcementJID, "announcement description", "announcement-prev")
		announcementInfo.IsDefaultSubGroup = true
		fake := &fakeGroupDescriptionClient{groupInfos: map[types.JID][]groupInfoResult{
			announcementJID: {{info: announcementInfo}},
		}}

		err := setGroupDescription(context.Background(), fake, announcementJID, "new description")
		if err == nil || !strings.Contains(err.Error(), "has no linked parent") {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(fake.sends) != 0 {
			t.Fatalf("description must not be sent without a parent: %#v", fake.sends)
		}
	})

	t.Run("linked group is not a parent", func(t *testing.T) {
		announcementInfo := testGroupInfo(announcementJID, "announcement description", "announcement-prev")
		announcementInfo.IsDefaultSubGroup = true
		announcementInfo.LinkedParentJID = parentJID
		fake := &fakeGroupDescriptionClient{groupInfos: map[types.JID][]groupInfoResult{
			announcementJID: {{info: announcementInfo}},
			parentJID:       {{info: testGroupInfo(parentJID, "not a parent", "parent-prev")}},
		}}

		err := setGroupDescription(context.Background(), fake, announcementJID, "new description")
		if err == nil || !strings.Contains(err.Error(), "is not a community parent") {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(fake.sends) != 0 {
			t.Fatalf("description must not be sent to an invalid parent: %#v", fake.sends)
		}
	})
}

func TestSetGroupDescriptionNoOpSkipsUpdate(t *testing.T) {
	parentJID := types.NewJID("parent", types.GroupServer)
	parentInfo := testGroupInfo(parentJID, "current description", "parent-prev")
	parentInfo.IsParent = true
	fake := &fakeGroupDescriptionClient{groupInfos: map[types.JID][]groupInfoResult{
		parentJID: {{info: parentInfo}},
	}}

	if err := setGroupDescription(context.Background(), fake, parentJID, "current description"); err != nil {
		t.Fatalf("SetGroupDescription returned an error: %v", err)
	}
	if len(fake.sends) != 0 {
		t.Fatalf("no-op description must not be sent: %#v", fake.sends)
	}
}

func TestSetGroupDescriptionRetriesConflictWithFreshPreviousID(t *testing.T) {
	groupJID := types.NewJID("group", types.GroupServer)
	fake := &fakeGroupDescriptionClient{
		groupInfos: map[types.JID][]groupInfoResult{groupJID: {
			{info: testGroupInfo(groupJID, "old description", "old-prev")},
			{info: testGroupInfo(groupJID, "concurrent description", "fresh-prev")},
		}},
		generatedIDs: []types.MessageID{"first-id", "retry-id"},
		sendErrors:   []error{&IQError{Code: 409, Text: "conflict"}, nil},
	}

	if err := setGroupDescription(context.Background(), fake, groupJID, "desired description"); err != nil {
		t.Fatalf("SetGroupDescription returned an error: %v", err)
	}
	if len(fake.sends) != 2 {
		t.Fatalf("unexpected send count: got %d, want 2", len(fake.sends))
	}
	if fake.sends[0].content.Attrs["prev"] != "old-prev" || fake.sends[1].content.Attrs["prev"] != "fresh-prev" {
		t.Fatalf("conflict retry did not refresh previous ID: first=%#v retry=%#v", fake.sends[0].content.Attrs, fake.sends[1].content.Attrs)
	}
	if fake.sends[0].content.Attrs["id"] != types.MessageID("first-id") || fake.sends[1].content.Attrs["id"] != types.MessageID("retry-id") {
		t.Fatalf("conflict retry did not regenerate description ID: first=%#v retry=%#v", fake.sends[0].content.Attrs, fake.sends[1].content.Attrs)
	}
}

func TestSetGroupDescriptionDoesNotRetryRateLimit(t *testing.T) {
	groupJID := types.NewJID("group", types.GroupServer)
	fake := &fakeGroupDescriptionClient{
		groupInfos: map[types.JID][]groupInfoResult{groupJID: {{info: testGroupInfo(groupJID, "old", "prev")}}},
		sendErrors: []error{ErrIQRateOverLimit},
	}

	err := setGroupDescription(context.Background(), fake, groupJID, "new")
	if !errors.Is(err, ErrIQRateOverLimit) {
		t.Fatalf("expected rate limit error, got %v", err)
	}
	if len(fake.sends) != 1 || len(fake.infoRequests) != 1 {
		t.Fatalf("rate limit must not be retried: info=%#v sends=%#v", fake.infoRequests, fake.sends)
	}
}
