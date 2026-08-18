// Copyright (c) 2026 Verbeux AI
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package whatsmeow

import (
	"context"
	"errors"
	"testing"

	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

type fakeCommunityGroupAddModeClient struct {
	iqType  infoQueryType
	jid     types.JID
	content waBinary.Node
	err     error
	calls   int
}

func (fake *fakeCommunityGroupAddModeClient) sendGroupIQ(_ context.Context, iqType infoQueryType, jid types.JID, content waBinary.Node) (*waBinary.Node, error) {
	fake.calls++
	fake.iqType = iqType
	fake.jid = jid
	fake.content = content
	return &waBinary.Node{}, fake.err
}

func TestSetCommunityGroupAddMode(t *testing.T) {
	community := types.NewJID("community", types.GroupServer)
	tests := []struct {
		name string
		mode types.CommunityGroupAddMode
		tag  string
	}{
		{name: "admins only", mode: types.CommunityGroupAddModeAdmin, tag: "not_allow_non_admin_sub_group_creation"},
		{name: "all members", mode: types.CommunityGroupAddModeAllMember, tag: "allow_non_admin_sub_group_creation"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeCommunityGroupAddModeClient{}
			if err := setCommunityGroupAddMode(context.Background(), fake, community, test.mode); err != nil {
				t.Fatalf("setCommunityGroupAddMode returned an error: %v", err)
			}
			if fake.calls != 1 || fake.iqType != iqSet || fake.jid != community || fake.content.Tag != test.tag {
				t.Fatalf("unexpected IQ: calls=%d type=%q jid=%s content=%#v", fake.calls, fake.iqType, fake.jid, fake.content)
			}
		})
	}
}

func TestSetCommunityGroupAddModeRejectsInvalidMode(t *testing.T) {
	fake := &fakeCommunityGroupAddModeClient{}
	err := setCommunityGroupAddMode(context.Background(), fake, types.NewJID("community", types.GroupServer), "invalid")
	if err == nil {
		t.Fatal("expected invalid mode error")
	}
	if fake.calls != 0 {
		t.Fatalf("invalid mode sent %d IQs, want 0", fake.calls)
	}
}

func TestSetCommunityGroupAddModePropagatesIQError(t *testing.T) {
	wantErr := errors.New("iq failed")
	fake := &fakeCommunityGroupAddModeClient{err: wantErr}
	err := setCommunityGroupAddMode(context.Background(), fake, types.NewJID("community", types.GroupServer), types.CommunityGroupAddModeAdmin)
	if !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want %v", err, wantErr)
	}
}

func TestParseGroupNodeCommunityGroupAddMode(t *testing.T) {
	client := &Client{}
	node := waBinary.Node{
		Tag:   "group",
		Attrs: waBinary.Attrs{"id": "community"},
		Content: []waBinary.Node{
			{Tag: "parent"},
			{Tag: "allow_non_admin_sub_group_creation"},
		},
	}

	info, err := client.parseGroupNode(&node)
	if err != nil {
		t.Fatalf("parseGroupNode returned an error: %v", err)
	}
	if !info.IsParent || !info.AllowNonAdminSubGroupCreation {
		t.Fatalf("unexpected community settings: %#v", info.GroupParent)
	}
}
