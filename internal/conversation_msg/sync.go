// Copyright © 2023 OpenIM SDK. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package conversation_msg

import (
	"context"
	"github.com/yrzs/openimsdkcore/pkg/common"
	"github.com/yrzs/openimsdkcore/pkg/constant"
	"github.com/yrzs/openimsdkcore/pkg/db/model_struct"
	"github.com/yrzs/openimsdkcore/pkg/syncer"
	utils2 "github.com/yrzs/openimsdktools/utils"
	"time"

	"github.com/yrzs/openimsdktools/log"
)

func (c *Conversation) SyncConversationsAndTriggerCallback(ctx context.Context, conversationsOnServer []*model_struct.LocalConversation) error {
	conversationsOnLocal, err := c.db.GetAllConversations(ctx)
	if err != nil {
		return err
	}
	if err := c.batchAddFaceURLAndName(ctx, conversationsOnServer...); err != nil {
		return err
	}
	if err = c.conversationSyncer.Sync(ctx, conversationsOnServer, conversationsOnLocal, func(ctx context.Context, state int, server, local *model_struct.LocalConversation) error {
		if state == syncer.Update || state == syncer.Insert {
			c.doUpdateConversation(common.Cmd2Value{Value: common.UpdateConNode{ConID: server.ConversationID, Action: constant.ConChange, Args: []string{server.ConversationID}}})
		}
		return nil
	}, true); err != nil {
		return err
	}
	return nil
}

func (c *Conversation) SyncConversations(ctx context.Context, conversationIDs []string) error {
	conversationsOnServer, err := c.getServerConversationsByIDs(ctx, conversationIDs)
	if err != nil {
		return err
	}
	return c.SyncConversationsAndTriggerCallback(ctx, conversationsOnServer)
}

func (c *Conversation) SyncAllConversations(ctx context.Context) error {
	ccTime := time.Now()
	conversationsOnServer, err := c.getServerConversationList(ctx)
	if err != nil {
		return err
	}
	log.ZDebug(ctx, "get server cost time", "cost time", time.Since(ccTime), "conversation on server", conversationsOnServer)
	return c.SyncConversationsAndTriggerCallback(ctx, conversationsOnServer)
}

func (c *Conversation) SyncAllConversationHashReadSeqs(ctx context.Context) error {
	log.ZDebug(ctx, "start SyncConversationHashReadSeqs")
	seqs, err := c.getServerHasReadAndMaxSeqs(ctx)
	if err != nil {
		return err
	}
	if len(seqs) == 0 {
		return nil
	}
	var conversationChangedIDs []string
	var conversationIDsNeedSync []string

	conversationsOnLocal, err := c.db.GetAllConversations(ctx)
	if err != nil {
		log.ZWarn(ctx, "get all conversations err", err)
		return err
	}
	conversationsOnLocalMap := utils2.SliceToMap(conversationsOnLocal, func(e *model_struct.LocalConversation) string {
		return e.ConversationID
	})
	for conversationID, v := range seqs {
		var unreadCount int32
		c.maxSeqRecorder.Set(conversationID, v.MaxSeq)
		if v.MaxSeq-v.HasReadSeq < 0 {
			unreadCount = 0
			log.ZWarn(ctx, "unread count is less than 0", nil, "conversationID",
				conversationID, "maxSeq", v.MaxSeq, "hasReadSeq", v.HasReadSeq)
		} else {
			unreadCount = int32(v.MaxSeq - v.HasReadSeq)
		}
		if conversation, ok := conversationsOnLocalMap[conversationID]; ok {
			if conversation.UnreadCount != unreadCount || conversation.HasReadSeq != v.HasReadSeq {
				if err := c.db.UpdateColumnsConversation(ctx, conversationID, map[string]interface{}{"unread_count": unreadCount, "has_read_seq": v.HasReadSeq}); err != nil {
					log.ZWarn(ctx, "UpdateColumnsConversation err", err, "conversationID", conversationID)
					continue
				}
				conversationChangedIDs = append(conversationChangedIDs, conversationID)
			}
		} else {
			conversationIDsNeedSync = append(conversationIDsNeedSync, conversationID)
		}

	}
	if len(conversationIDsNeedSync) > 0 {
		conversationsOnServer, err := c.getServerConversationsByIDs(ctx, conversationIDsNeedSync)
		if err != nil {
			log.ZWarn(ctx, "getServerConversationsByIDs err", err, "conversationIDs", conversationIDsNeedSync)
			return err
		}
		if err := c.batchAddFaceURLAndName(ctx, conversationsOnServer...); err != nil {
			log.ZWarn(ctx, "batchAddFaceURLAndName err", err, "conversationsOnServer", conversationsOnServer)
			return err
		}

		for _, conversation := range conversationsOnServer {
			var unreadCount int32
			v, ok := seqs[conversation.ConversationID]
			if !ok {
				continue
			}
			if v.MaxSeq-v.HasReadSeq < 0 {
				unreadCount = 0
				log.ZWarn(ctx, "unread count is less than 0", nil, "server seq", v, "conversation", conversation)
			} else {
				unreadCount = int32(v.MaxSeq - v.HasReadSeq)
			}
			conversation.UnreadCount = unreadCount
			conversation.HasReadSeq = v.HasReadSeq
		}
		err = c.db.BatchInsertConversationList(ctx, conversationsOnServer)
		if err != nil {
			log.ZWarn(ctx, "BatchInsertConversationList err", err, "conversationsOnServer", conversationsOnServer)
		}

	}

	log.ZDebug(ctx, "update conversations", "conversations", conversationChangedIDs)
	if len(conversationChangedIDs) > 0 {
		common.TriggerCmdUpdateConversation(ctx, common.UpdateConNode{Action: constant.ConChange, Args: conversationChangedIDs}, c.GetCh())
		common.TriggerCmdUpdateConversation(ctx, common.UpdateConNode{Action: constant.TotalUnreadMessageChanged}, c.GetCh())
	}
	return nil
}
