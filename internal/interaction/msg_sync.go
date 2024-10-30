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

package interaction

import (
	"context"
	"github.com/yrzs/openimsdkcore/pkg/common"
	"github.com/yrzs/openimsdkcore/pkg/constant"
	"github.com/yrzs/openimsdkcore/pkg/db/db_interface"
	"github.com/yrzs/openimsdkcore/sdk_struct"
	"strings"

	"github.com/yrzs/openimsdkprotocol/sdkws"
	"github.com/yrzs/openimsdktools/log"
)

const (
	connectPullNums = 1
	defaultPullNums = 10
	SplitPullMsgNum = 100
)

// The callback synchronization starts. The reconnection ends
type MsgSyncer struct {
	loginUserID        string                // login user ID
	longConnMgr        *LongConnMgr          // long connection manager
	PushMsgAndMaxSeqCh chan common.Cmd2Value // channel for receiving push messages and the maximum SEQ number
	conversationCh     chan common.Cmd2Value // storage and session triggering
	syncedMaxSeqs      map[string]int64      // map of the maximum synced SEQ numbers for all group IDs
	db                 db_interface.DataBase // data store
	syncTimes          int                   // times of sync
	ctx                context.Context       // context
	reinstalled        bool                  //true if the app was uninstalled and reinstalled

}

// NewMsgSyncer creates a new instance of the message synchronizer.
func NewMsgSyncer(ctx context.Context, conversationCh, PushMsgAndMaxSeqCh chan common.Cmd2Value,
	loginUserID string, longConnMgr *LongConnMgr, db db_interface.DataBase, syncTimes int) (*MsgSyncer, error) {
	m := &MsgSyncer{
		loginUserID:        loginUserID,
		longConnMgr:        longConnMgr,
		PushMsgAndMaxSeqCh: PushMsgAndMaxSeqCh,
		conversationCh:     conversationCh,
		ctx:                ctx,
		syncedMaxSeqs:      make(map[string]int64),
		db:                 db,
		syncTimes:          syncTimes,
	}
	if err := m.loadSeq(ctx); err != nil {
		log.ZError(ctx, "loadSeq err", err)
		return nil, err
	}
	return m, nil
}

// seq The db reads the data to the memory,set syncedMaxSeqs
func (m *MsgSyncer) loadSeq(ctx context.Context) error {
	conversationIDList, err := m.db.GetAllConversationIDList(ctx)
	if err != nil {
		log.ZError(ctx, "get conversation id list failed", err)
		return err
	}
	if len(conversationIDList) == 0 {
		m.reinstalled = true
	}
	for _, v := range conversationIDList {
		maxSyncedSeq, err := m.db.GetConversationNormalMsgSeq(ctx, v)
		if err != nil {
			log.ZError(ctx, "get group normal seq failed", err, "conversationID", v)
		} else {
			m.syncedMaxSeqs[v] = maxSyncedSeq
		}
	}
	notificationSeqs, err := m.db.GetNotificationAllSeqs(ctx)
	if err != nil {
		log.ZError(ctx, "get notification seq failed", err)
		return err
	}
	for _, notificationSeq := range notificationSeqs {
		m.syncedMaxSeqs[notificationSeq.ConversationID] = notificationSeq.Seq
	}
	log.ZDebug(ctx, "loadSeq", "syncedMaxSeqs", m.syncedMaxSeqs)
	return nil
}

// DoListener Listen to the message pipe of the message synchronizer
// and process received and pushed messages
func (m *MsgSyncer) DoListener(ctx context.Context) {
	for {
		select {
		case cmd := <-m.PushMsgAndMaxSeqCh:
			m.handlePushMsgAndEvent(cmd)
		case <-ctx.Done():
			log.ZInfo(m.ctx, "msg syncer done, sdk logout.....")
			return
		}
	}
}

// get seqs need sync interval
func (m *MsgSyncer) getSeqsNeedSync(syncedMaxSeq, maxSeq int64) []int64 {
	var seqs []int64
	for i := syncedMaxSeq + 1; i <= maxSeq; i++ {
		seqs = append(seqs, i)
	}
	return seqs
}

// recv msg from
func (m *MsgSyncer) handlePushMsgAndEvent(cmd common.Cmd2Value) {
	switch cmd.Cmd {
	case constant.CmdConnSuccesss:
		log.ZInfo(cmd.Ctx, "recv long conn mgr connected", "cmd", cmd.Cmd, "value", cmd.Value)
		m.doConnected(cmd.Ctx)
	case constant.CmdMaxSeq:
		log.ZInfo(cmd.Ctx, "recv max seqs from long conn mgr, start sync msgs", "cmd", cmd.Cmd, "value", cmd.Value)
		m.compareSeqsAndBatchSync(cmd.Ctx, cmd.Value.(*sdk_struct.CmdMaxSeqToMsgSync).ConversationMaxSeqOnSvr, defaultPullNums)
	case constant.CmdPushMsg:
		m.doPushMsg(cmd.Ctx, cmd.Value.(*sdkws.PushMessages))
	}
}

func (m *MsgSyncer) compareSeqsAndBatchSync(ctx context.Context, maxSeqToSync map[string]int64, pullNums int64) {
	needSyncSeqMap := make(map[string][2]int64)
	//when app reinstalled do not pull notifications messages.
	if m.reinstalled {
		notificationsSeqMap := make(map[string]int64)
		messagesSeqMap := make(map[string]int64)
		for conversationID, seq := range maxSeqToSync {
			if IsNotification(conversationID) {
				notificationsSeqMap[conversationID] = seq
			} else {
				messagesSeqMap[conversationID] = seq
			}
		}
		for conversationID, seq := range notificationsSeqMap {
			err := m.db.SetNotificationSeq(ctx, conversationID, seq)
			if err != nil {
				log.ZWarn(ctx, "SetNotificationSeq err", err, "conversationID", conversationID, "seq", seq)
				continue
			} else {
				m.syncedMaxSeqs[conversationID] = seq
			}
		}
		for conversationID, maxSeq := range messagesSeqMap {
			if syncedMaxSeq, ok := m.syncedMaxSeqs[conversationID]; ok {
				if maxSeq > syncedMaxSeq {
					needSyncSeqMap[conversationID] = [2]int64{syncedMaxSeq + 1, maxSeq}
				}
			} else {
				needSyncSeqMap[conversationID] = [2]int64{0, maxSeq}
			}
		}
		m.reinstalled = false
	} else {
		for conversationID, maxSeq := range maxSeqToSync {
			if syncedMaxSeq, ok := m.syncedMaxSeqs[conversationID]; ok {
				if maxSeq > syncedMaxSeq {
					needSyncSeqMap[conversationID] = [2]int64{syncedMaxSeq + 1, maxSeq}
				}
			} else {
				needSyncSeqMap[conversationID] = [2]int64{0, maxSeq}
			}
		}
	}
	_ = m.syncAndTriggerMsgs(m.ctx, needSyncSeqMap, pullNums)
}

func (m *MsgSyncer) doPushMsg(ctx context.Context, push *sdkws.PushMessages) {
	log.ZDebug(ctx, "push msgs", "push", push, "syncedMaxSeqs", m.syncedMaxSeqs)
	m.pushTriggerAndSync(ctx, push.Msgs, m.triggerConversation)
	m.pushTriggerAndSync(ctx, push.NotificationMsgs, m.triggerNotification)
}

func (m *MsgSyncer) pushTriggerAndSync(ctx context.Context, pullMsgs map[string]*sdkws.PullMsgs, triggerFunc func(ctx context.Context, msgs map[string]*sdkws.PullMsgs) error) {
	if len(pullMsgs) == 0 {
		return
	}
	needSyncSeqMap := make(map[string][2]int64)
	var lastSeq int64
	var storageMsgs []*sdkws.MsgData
	for conversationID, msgs := range pullMsgs {
		for _, msg := range msgs.Msgs {
			if msg.Seq == 0 {
				_ = triggerFunc(ctx, map[string]*sdkws.PullMsgs{conversationID: {Msgs: []*sdkws.MsgData{msg}}})
				continue
			}
			lastSeq = msg.Seq
			storageMsgs = append(storageMsgs, msg)
		}
		if lastSeq == m.syncedMaxSeqs[conversationID]+int64(len(storageMsgs)) && lastSeq != 0 {
			log.ZDebug(ctx, "trigger msgs", "msgs", storageMsgs)
			_ = triggerFunc(ctx, map[string]*sdkws.PullMsgs{conversationID: {Msgs: storageMsgs}})
			m.syncedMaxSeqs[conversationID] = lastSeq
		} else if lastSeq != 0 && lastSeq > m.syncedMaxSeqs[conversationID] {
			//must pull message when message type is notification
			needSyncSeqMap[conversationID] = [2]int64{m.syncedMaxSeqs[conversationID] + 1, lastSeq}
		}
	}
	m.syncAndTriggerMsgs(ctx, needSyncSeqMap, defaultPullNums)
}

// Called after successful reconnection to synchronize the latest message
func (m *MsgSyncer) doConnected(ctx context.Context) {
	common.TriggerCmdNotification(m.ctx, sdk_struct.CmdNewMsgComeToConversation{SyncFlag: constant.MsgSyncBegin}, m.conversationCh)
	var resp sdkws.GetMaxSeqResp
	if err := m.longConnMgr.SendReqWaitResp(m.ctx, &sdkws.GetMaxSeqReq{UserID: m.loginUserID}, constant.GetNewestSeq, &resp); err != nil {
		log.ZError(m.ctx, "get max seq error", err)
		common.TriggerCmdNotification(m.ctx, sdk_struct.CmdNewMsgComeToConversation{SyncFlag: constant.MsgSyncFailed}, m.conversationCh)
		return
	} else {
		log.ZDebug(m.ctx, "get max seq success", "resp", resp)
	}
	m.compareSeqsAndBatchSync(ctx, resp.MaxSeqs, connectPullNums)
	common.TriggerCmdNotification(m.ctx, sdk_struct.CmdNewMsgComeToConversation{SyncFlag: constant.MsgSyncEnd}, m.conversationCh)
}

func IsNotification(conversationID string) bool {
	return strings.HasPrefix(conversationID, "n_")
}

// Fragment synchronization message, seq refresh after successful trigger
func (m *MsgSyncer) syncAndTriggerMsgs(ctx context.Context, seqMap map[string][2]int64, syncMsgNum int64) error {
	if len(seqMap) > 0 {
		log.ZDebug(ctx, "current sync seqMap", "seqMap", seqMap)
		tempSeqMap := make(map[string][2]int64, 50)
		msgNum := 0
		for k, v := range seqMap {
			oneConversationSyncNum := v[1] - v[0] + 1
			if (oneConversationSyncNum/SplitPullMsgNum) > 1 && IsNotification(k) {
				nSeqMap := make(map[string][2]int64, 1)
				count := int(oneConversationSyncNum / SplitPullMsgNum)
				startSeq := v[0]
				var end int64
				for i := 0; i <= count; i++ {
					if i == count {
						nSeqMap[k] = [2]int64{startSeq, v[1]}
					} else {
						end = startSeq + int64(SplitPullMsgNum)
						if end > v[1] {
							end = v[1]
							i = count
						}
						nSeqMap[k] = [2]int64{startSeq, end}
					}
					resp, err := m.pullMsgBySeqRange(ctx, nSeqMap, syncMsgNum)
					if err != nil {
						log.ZError(ctx, "syncMsgFromSvr err", err, "nSeqMap", nSeqMap)
						return err
					}
					_ = m.triggerConversation(ctx, resp.Msgs)
					_ = m.triggerNotification(ctx, resp.NotificationMsgs)
					for conversationID, seqs := range nSeqMap {
						m.syncedMaxSeqs[conversationID] = seqs[1]
					}
					startSeq = end + 1
				}
				continue
			}
			tempSeqMap[k] = v
			if oneConversationSyncNum > 0 {
				msgNum += int(oneConversationSyncNum)
			}
			if msgNum >= SplitPullMsgNum {
				resp, err := m.pullMsgBySeqRange(ctx, tempSeqMap, syncMsgNum)
				if err != nil {
					log.ZError(ctx, "syncMsgFromSvr err", err, "tempSeqMap", tempSeqMap)
					return err
				}
				_ = m.triggerConversation(ctx, resp.Msgs)
				_ = m.triggerNotification(ctx, resp.NotificationMsgs)
				for conversationID, seqs := range tempSeqMap {
					m.syncedMaxSeqs[conversationID] = seqs[1]
				}
				tempSeqMap = make(map[string][2]int64, 50)
				msgNum = 0
			}
		}

		resp, err := m.pullMsgBySeqRange(ctx, tempSeqMap, syncMsgNum)
		if err != nil {
			log.ZError(ctx, "syncMsgFromSvr err", err, "seqMap", seqMap)
			return err
		}
		_ = m.triggerConversation(ctx, resp.Msgs)
		_ = m.triggerNotification(ctx, resp.NotificationMsgs)
		for conversationID, seqs := range seqMap {
			m.syncedMaxSeqs[conversationID] = seqs[1]
		}
	} else {
		log.ZDebug(ctx, "noting conversation to sync", "syncMsgNum", syncMsgNum)
	}
	return nil
}

func (m *MsgSyncer) splitSeqs(split int, seqsNeedSync []int64) (splitSeqs [][]int64) {
	if len(seqsNeedSync) <= split {
		splitSeqs = append(splitSeqs, seqsNeedSync)
		return
	}
	for i := 0; i < len(seqsNeedSync); i += split {
		end := i + split
		if end > len(seqsNeedSync) {
			end = len(seqsNeedSync)
		}
		splitSeqs = append(splitSeqs, seqsNeedSync[i:end])
	}
	return
}

func (m *MsgSyncer) pullMsgBySeqRange(ctx context.Context, seqMap map[string][2]int64, syncMsgNum int64) (resp *sdkws.PullMessageBySeqsResp, err error) {
	log.ZDebug(ctx, "pullMsgBySeqRange", "seqMap", seqMap, "syncMsgNum", syncMsgNum)

	req := sdkws.PullMessageBySeqsReq{UserID: m.loginUserID}
	for conversationID, seqs := range seqMap {
		req.SeqRanges = append(req.SeqRanges, &sdkws.SeqRange{
			ConversationID: conversationID,
			Begin:          seqs[0],
			End:            seqs[1],
			Num:            syncMsgNum,
		})
	}
	resp = &sdkws.PullMessageBySeqsResp{}
	if err := m.longConnMgr.SendReqWaitResp(ctx, &req, constant.PullMsgBySeqList, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// synchronizes messages by SEQs.
func (m *MsgSyncer) syncMsgBySeqs(ctx context.Context, conversationID string, seqsNeedSync []int64) (allMsgs []*sdkws.MsgData, err error) {
	pullMsgReq := sdkws.PullMessageBySeqsReq{}
	pullMsgReq.UserID = m.loginUserID
	split := constant.SplitPullMsgNum
	seqsList := m.splitSeqs(split, seqsNeedSync)
	for i := 0; i < len(seqsList); {
		var pullMsgResp sdkws.PullMessageBySeqsResp
		err := m.longConnMgr.SendReqWaitResp(ctx, &pullMsgReq, constant.PullMsgBySeqList, &pullMsgResp)
		if err != nil {
			log.ZError(ctx, "syncMsgFromSvrSplit err", err, "pullMsgReq", pullMsgReq)
			continue
		}
		i++
		allMsgs = append(allMsgs, pullMsgResp.Msgs[conversationID].Msgs...)
	}
	return allMsgs, nil
}

// triggers a conversation with a new message.
func (m *MsgSyncer) triggerConversation(ctx context.Context, msgs map[string]*sdkws.PullMsgs) error {
	if len(msgs) >= 0 {
		err := common.TriggerCmdNewMsgCome(ctx, sdk_struct.CmdNewMsgComeToConversation{Msgs: msgs}, m.conversationCh)
		if err != nil {
			log.ZError(ctx, "triggerCmdNewMsgCome err", err, "msgs", msgs)
		}
		log.ZDebug(ctx, "triggerConversation", "msgs", msgs)
		return err
	}
	return nil
}

func (m *MsgSyncer) triggerNotification(ctx context.Context, msgs map[string]*sdkws.PullMsgs) error {
	if len(msgs) >= 0 {
		err := common.TriggerCmdNotification(ctx, sdk_struct.CmdNewMsgComeToConversation{Msgs: msgs}, m.conversationCh)
		if err != nil {
			log.ZError(ctx, "triggerCmdNewMsgCome err", err, "msgs", msgs)
		}
		return err
	}
	return nil

}