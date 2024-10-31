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
	"encoding/json"
	"github.com/yrzs/openimsdkcore/internal/util"
	"github.com/yrzs/openimsdkcore/pkg/common"
	"github.com/yrzs/openimsdkcore/pkg/constant"
	"github.com/yrzs/openimsdkcore/pkg/db/db_interface"
	"github.com/yrzs/openimsdkcore/pkg/db/model_struct"
	"github.com/yrzs/openimsdkcore/pkg/utils"
	"github.com/yrzs/openimsdkcore/sdk_struct"

	"github.com/openimsdk/protocol/sdkws"
	"github.com/yrzs/openimsdktools/log"
)

type MessageController struct {
	db db_interface.DataBase
	ch chan common.Cmd2Value
}

func NewMessageController(db db_interface.DataBase, ch chan common.Cmd2Value) *MessageController {
	return &MessageController{db: db, ch: ch}
}
func (m *MessageController) BatchUpdateMessageList(ctx context.Context, updateMsg map[string][]*model_struct.LocalChatLog) error {
	if updateMsg == nil {
		return nil
	}
	for conversationID, messages := range updateMsg {
		conversation, err := m.db.GetConversation(ctx, conversationID)
		if err != nil {
			log.ZError(ctx, "GetConversation err", err, "conversationID", conversationID)
			continue
		}
		latestMsg := &sdk_struct.MsgStruct{}
		if err := json.Unmarshal([]byte(conversation.LatestMsg), latestMsg); err != nil {
			log.ZError(ctx, "Unmarshal err", err, "conversationID",
				conversationID, "latestMsg", conversation.LatestMsg)
			continue
		}
		for _, v := range messages {
			v1 := new(model_struct.LocalChatLog)
			v1.ClientMsgID = v.ClientMsgID
			v1.Seq = v.Seq
			v1.Status = v.Status
			v1.RecvID = v.RecvID
			v1.SessionType = v.SessionType
			v1.ServerMsgID = v.ServerMsgID
			v1.SendTime = v.SendTime
			err := m.db.UpdateMessage(ctx, conversationID, v1)
			if err != nil {
				return utils.Wrap(err, "BatchUpdateMessageList failed")
			}
			if latestMsg.ClientMsgID == v.ClientMsgID {
				latestMsg.ServerMsgID = v.ServerMsgID
				latestMsg.Seq = v.Seq
				latestMsg.SendTime = v.SendTime
				conversation.LatestMsg = utils.StructToJsonString(latestMsg)
				_ = common.TriggerCmdUpdateConversation(ctx, common.UpdateConNode{ConID: conversation.ConversationID,
					Action: constant.AddConOrUpLatMsg, Args: *conversation}, m.ch)

			}
		}

	}
	return nil
}

func (m *MessageController) BatchInsertMessageList(ctx context.Context, insertMsg map[string][]*model_struct.LocalChatLog) error {
	if insertMsg == nil {
		return nil
	}
	for conversationID, messages := range insertMsg {
		if len(messages) == 0 {
			continue
		}
		err := m.db.BatchInsertMessageList(ctx, conversationID, messages)
		if err != nil {
			log.ZError(ctx, "insert GetMessage detail err:", err, "conversationID", conversationID, "messages", messages)
			for _, v := range messages {
				e := m.db.InsertMessage(ctx, conversationID, v)
				if e != nil {
					log.ZError(ctx, "InsertMessage err", err, "conversationID", conversationID, "message", v)
				}
			}
		}

	}
	return nil
}

func (c *Conversation) PullMessageBySeqs(ctx context.Context, seqs []*sdkws.SeqRange) (*sdkws.PullMessageBySeqsResp, error) {
	return util.CallApi[sdkws.PullMessageBySeqsResp](ctx, constant.PullUserMsgBySeqRouter, sdkws.PullMessageBySeqsReq{UserID: c.loginUserID, SeqRanges: seqs})
}
func (m *MessageController) SearchMessageByContentTypeAndKeyword(ctx context.Context, contentType []int, keywordList []string,
	keywordListMatchType int, startTime, endTime int64) (result []*model_struct.LocalChatLog, err error) {
	var list []*model_struct.LocalChatLog
	conversationIDList, err := m.db.GetAllConversationIDList(ctx)
	for _, v := range conversationIDList {
		sList, err := m.db.SearchMessageByContentTypeAndKeyword(ctx, contentType, v, keywordList, keywordListMatchType, startTime, endTime)
		if err != nil {
			// TODO: log.Error(operationID, "search message in group err", err.Error(), v)
			continue
		}
		list = append(list, sList...)
	}

	return list, nil
}
