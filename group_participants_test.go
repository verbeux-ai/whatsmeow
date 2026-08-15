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

type communityParticipantUpdateCall struct {
	jid          types.JID
	participants []types.JID
	action       ParticipantChange
	actionAttrs  waBinary.Attrs
}

type fakeCommunityParticipantUpdater struct {
	subGroups        []*types.GroupLinkTarget
	subGroupsErr     error
	subGroupRequests []types.JID
	updateCalls      []communityParticipantUpdateCall
	result           []types.GroupParticipant
	updateErr        error
}

type fakeCommunityAwareParticipantUpdater struct {
	groupData        *groupMetaCache
	groupDataErr     error
	metadataRequests []types.JID
	groupCalls       []communityParticipantUpdateCall
	communityCalls   []communityParticipantUpdateCall
}

func (fake *fakeCommunityParticipantUpdater) GetSubGroups(_ context.Context, community types.JID) ([]*types.GroupLinkTarget, error) {
	fake.subGroupRequests = append(fake.subGroupRequests, community)
	return fake.subGroups, fake.subGroupsErr
}

func (fake *fakeCommunityParticipantUpdater) updateGroupParticipants(_ context.Context, jid types.JID, participants []types.JID, action ParticipantChange, actionAttrs waBinary.Attrs) ([]types.GroupParticipant, error) {
	fake.updateCalls = append(fake.updateCalls, communityParticipantUpdateCall{
		jid:          jid,
		participants: participants,
		action:       action,
		actionAttrs:  actionAttrs,
	})
	return fake.result, fake.updateErr
}

func (fake *fakeCommunityAwareParticipantUpdater) getCachedGroupData(_ context.Context, jid types.JID) (*groupMetaCache, error) {
	fake.metadataRequests = append(fake.metadataRequests, jid)
	return fake.groupData, fake.groupDataErr
}

func (fake *fakeCommunityAwareParticipantUpdater) UpdateGroupParticipants(_ context.Context, jid types.JID, participants []types.JID, action ParticipantChange) ([]types.GroupParticipant, error) {
	fake.groupCalls = append(fake.groupCalls, communityParticipantUpdateCall{jid: jid, participants: participants, action: action})
	return nil, nil
}

func (fake *fakeCommunityAwareParticipantUpdater) UpdateCommunityParticipants(_ context.Context, jid types.JID, participants []types.JID, action ParticipantChange) ([]types.GroupParticipant, error) {
	fake.communityCalls = append(fake.communityCalls, communityParticipantUpdateCall{jid: jid, participants: participants, action: action})
	return nil, nil
}

func TestUpdateCommunityParticipantsAddUsesAnnouncementGroup(t *testing.T) {
	community := types.NewJID("community", types.GroupServer)
	regularSubGroup := types.NewJID("regular", types.GroupServer)
	announcementGroup := types.NewJID("announcement", types.GroupServer)
	participant := types.NewJID("participant", types.DefaultUserServer)
	want := []types.GroupParticipant{{JID: participant}}
	fake := &fakeCommunityParticipantUpdater{
		subGroups: []*types.GroupLinkTarget{
			{JID: regularSubGroup},
			{JID: announcementGroup, GroupIsDefaultSub: types.GroupIsDefaultSub{IsDefaultSubGroup: true}},
		},
		result: want,
	}

	got, err := updateCommunityParticipants(context.Background(), fake, community, []types.JID{participant}, ParticipantChangeAdd)
	if err != nil {
		t.Fatalf("UpdateCommunityParticipants returned an error: %v", err)
	}
	if len(fake.subGroupRequests) != 1 || fake.subGroupRequests[0] != community {
		t.Fatalf("unexpected subgroup requests: %#v", fake.subGroupRequests)
	}
	if len(fake.updateCalls) != 1 {
		t.Fatalf("unexpected update call count: got %d, want 1", len(fake.updateCalls))
	}
	call := fake.updateCalls[0]
	if call.jid != announcementGroup || call.action != ParticipantChangeAdd {
		t.Fatalf("add sent to wrong target: jid=%s action=%s", call.jid, call.action)
	}
	if len(call.actionAttrs) != 0 {
		t.Fatalf("add must not contain community removal attributes: %#v", call.actionAttrs)
	}
	if len(got) != len(want) || got[0].JID != want[0].JID {
		t.Fatalf("unexpected participant response: %#v", got)
	}
}

func TestUpdateCommunityParticipantsAddRequiresAnnouncementGroup(t *testing.T) {
	community := types.NewJID("community", types.GroupServer)
	fake := &fakeCommunityParticipantUpdater{
		subGroups: []*types.GroupLinkTarget{{JID: types.NewJID("regular", types.GroupServer)}},
	}

	_, err := updateCommunityParticipants(context.Background(), fake, community, nil, ParticipantChangeAdd)
	if err == nil {
		t.Fatal("expected an error when the community has no announcement group")
	}
	if len(fake.updateCalls) != 0 {
		t.Fatalf("participant update must not be sent without an announcement group: %#v", fake.updateCalls)
	}
}

func TestUpdateCommunityParticipantsAddPropagatesSubGroupError(t *testing.T) {
	community := types.NewJID("community", types.GroupServer)
	wantErr := errors.New("subgroups unavailable")
	fake := &fakeCommunityParticipantUpdater{subGroupsErr: wantErr}

	_, err := updateCommunityParticipants(context.Background(), fake, community, nil, ParticipantChangeAdd)
	if !errors.Is(err, wantErr) {
		t.Fatalf("unexpected error: got %v, want wrapped %v", err, wantErr)
	}
}

func TestUpdateCommunityParticipantsRemoveUsesParentAndLinkedGroups(t *testing.T) {
	community := types.NewJID("community", types.GroupServer)
	participant := types.NewJID("participant", types.DefaultUserServer)
	fake := &fakeCommunityParticipantUpdater{}

	_, err := updateCommunityParticipants(context.Background(), fake, community, []types.JID{participant}, ParticipantChangeRemove)
	if err != nil {
		t.Fatalf("UpdateCommunityParticipants returned an error: %v", err)
	}
	if len(fake.subGroupRequests) != 0 {
		t.Fatalf("remove unexpectedly queried subgroups: %#v", fake.subGroupRequests)
	}
	if len(fake.updateCalls) != 1 {
		t.Fatalf("unexpected update call count: got %d, want 1", len(fake.updateCalls))
	}
	call := fake.updateCalls[0]
	if call.jid != community || call.action != ParticipantChangeRemove {
		t.Fatalf("remove sent to wrong target: jid=%s action=%s", call.jid, call.action)
	}
	if call.actionAttrs["linked_groups"] != "true" {
		t.Fatalf("remove is missing linked_groups=true: %#v", call.actionAttrs)
	}
}

func TestUpdateCommunityParticipantsAdminChangesUseParent(t *testing.T) {
	community := types.NewJID("community", types.GroupServer)
	participant := types.NewJID("participant", types.DefaultUserServer)

	for _, action := range []ParticipantChange{ParticipantChangePromote, ParticipantChangeDemote} {
		t.Run(string(action), func(t *testing.T) {
			fake := &fakeCommunityParticipantUpdater{}
			_, err := updateCommunityParticipants(context.Background(), fake, community, []types.JID{participant}, action)
			if err != nil {
				t.Fatalf("UpdateCommunityParticipants returned an error: %v", err)
			}
			if len(fake.subGroupRequests) != 0 {
				t.Fatalf("%s unexpectedly queried subgroups: %#v", action, fake.subGroupRequests)
			}
			if len(fake.updateCalls) != 1 {
				t.Fatalf("unexpected update call count: got %d, want 1", len(fake.updateCalls))
			}
			call := fake.updateCalls[0]
			if call.jid != community || call.action != action {
				t.Fatalf("%s sent to wrong target: jid=%s action=%s", action, call.jid, call.action)
			}
			if len(call.actionAttrs) != 0 {
				t.Fatalf("%s must not contain removal attributes: %#v", action, call.actionAttrs)
			}
		})
	}
}

func TestUpdateGroupParticipantsWithCommunityUsesCommunityProtocolForParent(t *testing.T) {
	community := types.NewJID("community", types.GroupServer)
	participant := types.NewJID("participant", types.DefaultUserServer)
	fake := &fakeCommunityAwareParticipantUpdater{groupData: &groupMetaCache{CommunityParent: true}}

	_, err := updateGroupParticipantsWithCommunity(context.Background(), fake, community, []types.JID{participant}, ParticipantChangeAdd)
	if err != nil {
		t.Fatalf("UpdateGroupParticipantsWithCommunity returned an error: %v", err)
	}
	if len(fake.metadataRequests) != 1 || fake.metadataRequests[0] != community {
		t.Fatalf("unexpected metadata requests: %#v", fake.metadataRequests)
	}
	if len(fake.communityCalls) != 1 || len(fake.groupCalls) != 0 {
		t.Fatalf("unexpected dispatch: community=%#v group=%#v", fake.communityCalls, fake.groupCalls)
	}
}

func TestUpdateGroupParticipantsWithCommunityKeepsOrdinaryGroupProtocol(t *testing.T) {
	group := types.NewJID("group", types.GroupServer)
	participant := types.NewJID("participant", types.DefaultUserServer)
	fake := &fakeCommunityAwareParticipantUpdater{groupData: &groupMetaCache{}}

	_, err := updateGroupParticipantsWithCommunity(context.Background(), fake, group, []types.JID{participant}, ParticipantChangeAdd)
	if err != nil {
		t.Fatalf("UpdateGroupParticipantsWithCommunity returned an error: %v", err)
	}
	if len(fake.groupCalls) != 1 || len(fake.communityCalls) != 0 {
		t.Fatalf("unexpected dispatch: group=%#v community=%#v", fake.groupCalls, fake.communityCalls)
	}
}

func TestUpdateGroupParticipantsWithCommunityKeepsAnnouncementGroupProtocol(t *testing.T) {
	announcement := types.NewJID("announcement", types.GroupServer)
	participant := types.NewJID("participant", types.DefaultUserServer)
	fake := &fakeCommunityAwareParticipantUpdater{groupData: &groupMetaCache{CommunityAnnouncementGroup: true}}

	_, err := updateGroupParticipantsWithCommunity(context.Background(), fake, announcement, []types.JID{participant}, ParticipantChangeRemove)
	if err != nil {
		t.Fatalf("UpdateGroupParticipantsWithCommunity returned an error: %v", err)
	}
	if len(fake.groupCalls) != 1 || len(fake.communityCalls) != 0 {
		t.Fatalf("unexpected dispatch: group=%#v community=%#v", fake.groupCalls, fake.communityCalls)
	}
	if fake.groupCalls[0].jid != announcement || fake.groupCalls[0].action != ParticipantChangeRemove {
		t.Fatalf("announcement group call changed unexpectedly: %#v", fake.groupCalls[0])
	}
}

func TestUpdateGroupParticipantsWithCommunityPropagatesMetadataError(t *testing.T) {
	group := types.NewJID("group", types.GroupServer)
	wantErr := errors.New("metadata unavailable")
	fake := &fakeCommunityAwareParticipantUpdater{groupDataErr: wantErr}

	_, err := updateGroupParticipantsWithCommunity(context.Background(), fake, group, nil, ParticipantChangeAdd)
	if !errors.Is(err, wantErr) {
		t.Fatalf("unexpected error: got %v, want wrapped %v", err, wantErr)
	}
	if len(fake.groupCalls) != 0 || len(fake.communityCalls) != 0 {
		t.Fatalf("participant update must not be sent without metadata: group=%#v community=%#v", fake.groupCalls, fake.communityCalls)
	}
}
