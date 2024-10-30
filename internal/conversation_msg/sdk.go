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
	"fmt"
	"github.com/yrzs/openimsdkcore/internal/file"
	"github.com/yrzs/openimsdkcore/open_im_sdk_callback"
	"github.com/yrzs/openimsdkcore/pkg/common"
	"github.com/yrzs/openimsdkcore/pkg/constant"
	"github.com/yrzs/openimsdkcore/pkg/content_type"
	"github.com/yrzs/openimsdkcore/pkg/db/model_struct"
	"github.com/yrzs/openimsdkcore/pkg/sdk_params_callback"
	"github.com/yrzs/openimsdkcore/pkg/sdkerrs"
	"github.com/yrzs/openimsdkcore/pkg/server_api_params"
	"github.com/yrzs/openimsdkcore/pkg/utils"
	"github.com/yrzs/openimsdkcore/sdk_struct"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yrzs/openimsdktools/log"

	pbConversation "github.com/yrzs/openimsdkprotocol/conversation"
	"github.com/yrzs/openimsdkprotocol/sdkws"
	"github.com/yrzs/openimsdkprotocol/wrapperspb"

	"github.com/jinzhu/copier"
)

func (c *Conversation) GetAllConversationList(ctx context.Context) ([]*model_struct.LocalConversation, error) {
	return c.db.GetAllConversationListDB(ctx)
}

func (c *Conversation) GetConversationListSplit(ctx context.Context, offset, count int) ([]*model_struct.LocalConversation, error) {
	return c.db.GetConversationListSplitDB(ctx, offset, count)
}

func (c *Conversation) HideConversation(ctx context.Context, conversationID string) error {
	err := c.db.ResetConversation(ctx, conversationID)
	if err != nil {
		return err
	}
	return nil
}

func (c *Conversation) GetAtAllTag(_ context.Context) string {
	return constant.AtAllString
}

// deprecated
func (c *Conversation) GetConversationRecvMessageOpt(ctx context.Context, conversationIDs []string) (resp []*server_api_params.GetConversationRecvMessageOptResp, err error) {
	conversations, err := c.db.GetMultipleConversationDB(ctx, conversationIDs)
	if err != nil {
		return nil, err
	}
	for _, conversation := range conversations {
		resp = append(resp, &server_api_params.GetConversationRecvMessageOptResp{
			ConversationID: conversation.ConversationID,
			Result:         &conversation.RecvMsgOpt,
		})
	}
	return resp, nil
}

// Method to set global message receiving options
func (c *Conversation) GetOneConversation(ctx context.Context, sessionType int32, sourceID string) (*model_struct.LocalConversation, error) {
	conversationID := c.getConversationIDBySessionType(sourceID, int(sessionType))
	lc, err := c.db.GetConversation(ctx, conversationID)
	if err == nil {
		return lc, nil
	} else {
		var newConversation model_struct.LocalConversation
		newConversation.ConversationID = conversationID
		newConversation.ConversationType = sessionType
		switch sessionType {
		case constant.SingleChatType:
			newConversation.UserID = sourceID
			faceUrl, name, err := c.getUserNameAndFaceURL(ctx, sourceID)
			if err != nil {
				return nil, err
			}
			newConversation.ShowName = name
			newConversation.FaceURL = faceUrl
		case constant.GroupChatType, constant.SuperGroupChatType:
			newConversation.GroupID = sourceID
			g, err := c.full.GetGroupInfoFromLocal2Svr(ctx, sourceID, sessionType)
			if err != nil {
				return nil, err
			}
			newConversation.ShowName = g.GroupName
			newConversation.FaceURL = g.FaceURL
		}
		time.Sleep(time.Millisecond * 500)
		lc, errTemp := c.db.GetConversation(ctx, conversationID)
		if errTemp == nil {
			return lc, nil
		}
		err := c.db.InsertConversation(ctx, &newConversation)
		if err != nil {
			return nil, err
		}
		return &newConversation, nil
	}
}

func (c *Conversation) GetMultipleConversation(ctx context.Context, conversationIDList []string) ([]*model_struct.LocalConversation, error) {
	conversations, err := c.db.GetMultipleConversationDB(ctx, conversationIDList)
	if err != nil {
		return nil, err
	}
	return conversations, nil

}

func (c *Conversation) HideAllConversations(ctx context.Context) error {
	err := c.db.ResetAllConversation(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (c *Conversation) SetConversationDraft(ctx context.Context, conversationID, draftText string) error {
	if draftText != "" {
		err := c.db.SetConversationDraftDB(ctx, conversationID, draftText)
		if err != nil {
			return err
		}
	} else {
		err := c.db.RemoveConversationDraft(ctx, conversationID, draftText)
		if err != nil {
			return err
		}
	}
	_ = common.TriggerCmdUpdateConversation(ctx, common.UpdateConNode{Action: constant.ConChange, Args: []string{conversationID}}, c.GetCh())
	return nil
}

func (c *Conversation) setConversationAndSync(ctx context.Context, conversationID string, req *pbConversation.ConversationReq) error {
	lc, err := c.db.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}
	apiReq := &pbConversation.SetConversationsReq{Conversation: req}
	err = c.setConversation(ctx, apiReq, lc)
	if err != nil {
		return err
	}
	c.SyncConversations(ctx, []string{conversationID})
	return nil
}

func (c *Conversation) ResetConversationGroupAtType(ctx context.Context, conversationID string) error {
	return c.setConversationAndSync(ctx, conversationID, &pbConversation.ConversationReq{GroupAtType: &wrapperspb.Int32Value{Value: 0}})
}

func (c *Conversation) PinConversation(ctx context.Context, conversationID string, isPinned bool) error {
	return c.setConversationAndSync(ctx, conversationID, &pbConversation.ConversationReq{IsPinned: &wrapperspb.BoolValue{Value: isPinned}})
}

func (c *Conversation) SetOneConversationPrivateChat(ctx context.Context, conversationID string, isPrivate bool) error {
	return c.setConversationAndSync(ctx, conversationID, &pbConversation.ConversationReq{IsPrivateChat: &wrapperspb.BoolValue{Value: isPrivate}})
}

func (c *Conversation) SetConversationMsgDestructTime(ctx context.Context, conversationID string, msgDestructTime int64) error {
	return c.setConversationAndSync(ctx, conversationID, &pbConversation.ConversationReq{MsgDestructTime: &wrapperspb.Int64Value{Value: msgDestructTime}})
}

func (c *Conversation) SetConversationIsMsgDestruct(ctx context.Context, conversationID string, isMsgDestruct bool) error {
	return c.setConversationAndSync(ctx, conversationID, &pbConversation.ConversationReq{IsMsgDestruct: &wrapperspb.BoolValue{Value: isMsgDestruct}})
}

func (c *Conversation) SetOneConversationBurnDuration(ctx context.Context, conversationID string, burnDuration int32) error {
	return c.setConversationAndSync(ctx, conversationID, &pbConversation.ConversationReq{BurnDuration: &wrapperspb.Int32Value{Value: burnDuration}})
}

func (c *Conversation) SetOneConversationRecvMessageOpt(ctx context.Context, conversationID string, opt int) error {
	return c.setConversationAndSync(ctx, conversationID, &pbConversation.ConversationReq{RecvMsgOpt: &wrapperspb.Int32Value{Value: int32(opt)}})
}
func (c *Conversation) SetOneConversationEx(ctx context.Context, conversationID string, ex string) error {
	return c.setConversationAndSync(ctx, conversationID, &pbConversation.ConversationReq{Ex: &wrapperspb.StringValue{
		Value: ex,
	}})
}
func (c *Conversation) GetTotalUnreadMsgCount(ctx context.Context) (totalUnreadCount int32, err error) {
	return c.db.GetTotalUnreadMsgCountDB(ctx)
}

func (c *Conversation) SetConversationListener(listener func() open_im_sdk_callback.OnConversationListener) {
	c.ConversationListener = listener
}

func (c *Conversation) msgStructToLocalChatLog(src *sdk_struct.MsgStruct) *model_struct.LocalChatLog {
	var lc model_struct.LocalChatLog
	copier.Copy(&lc, src)
	switch src.ContentType {
	case constant.Text:
		lc.Content = utils.StructToJsonString(src.TextElem)
	case constant.Picture:
		lc.Content = utils.StructToJsonString(src.PictureElem)
	case constant.Sound:
		lc.Content = utils.StructToJsonString(src.SoundElem)
	case constant.Video:
		lc.Content = utils.StructToJsonString(src.VideoElem)
	case constant.File:
		lc.Content = utils.StructToJsonString(src.FileElem)
	case constant.AtText:
		lc.Content = utils.StructToJsonString(src.AtTextElem)
	case constant.Merger:
		lc.Content = utils.StructToJsonString(src.MergeElem)
	case constant.Card:
		lc.Content = utils.StructToJsonString(src.CardElem)
	case constant.Location:
		lc.Content = utils.StructToJsonString(src.LocationElem)
	case constant.Custom:
		lc.Content = utils.StructToJsonString(src.CustomElem)
	case constant.Quote:
		lc.Content = utils.StructToJsonString(src.QuoteElem)
	case constant.Face:
		lc.Content = utils.StructToJsonString(src.FaceElem)
	case constant.AdvancedText:
		lc.Content = utils.StructToJsonString(src.AdvancedTextElem)
	default:
		lc.Content = utils.StructToJsonString(src.NotificationElem)
	}
	if src.SessionType == constant.GroupChatType || src.SessionType == constant.SuperGroupChatType {
		lc.RecvID = src.GroupID
	}
	lc.AttachedInfo = utils.StructToJsonString(src.AttachedInfoElem)
	return &lc
}
func (c *Conversation) msgDataToLocalChatLog(src *sdkws.MsgData) *model_struct.LocalChatLog {
	var lc model_struct.LocalChatLog
	copier.Copy(&lc, src)
	lc.Content = string(src.Content)
	if src.SessionType == constant.GroupChatType || src.SessionType == constant.SuperGroupChatType {
		lc.RecvID = src.GroupID

	}
	return &lc

}
func (c *Conversation) msgDataToLocalErrChatLog(src *model_struct.LocalChatLog) *model_struct.LocalErrChatLog {
	var lc model_struct.LocalErrChatLog
	copier.Copy(&lc, src)
	return &lc

}

func localChatLogToMsgStruct(dst *sdk_struct.NewMsgList, src []*model_struct.LocalChatLog) {
	copier.Copy(dst, &src)

}

func (c *Conversation) updateMsgStatusAndTriggerConversation(ctx context.Context, clientMsgID, serverMsgID string, sendTime int64, status int32, s *sdk_struct.MsgStruct,
	lc *model_struct.LocalConversation, isOnlineOnly bool) {
	log.ZDebug(ctx, "this is test send message ", "sendTime", sendTime, "status", status, "clientMsgID", clientMsgID, "serverMsgID", serverMsgID)
	if isOnlineOnly {
		return
	}
	s.SendTime = sendTime
	s.Status = status
	s.ServerMsgID = serverMsgID
	err := c.db.UpdateMessageTimeAndStatus(ctx, lc.ConversationID, clientMsgID, serverMsgID, sendTime, status)
	if err != nil {
		log.ZWarn(ctx, "send message update message status error", err,
			"sendTime", sendTime, "status", status, "clientMsgID", clientMsgID, "serverMsgID", serverMsgID)
	}
	err = c.db.DeleteSendingMessage(ctx, lc.ConversationID, clientMsgID)
	if err != nil {
		log.ZWarn(ctx, "send message delete sending message error", err)
	}
	lc.LatestMsg = utils.StructToJsonString(s)
	lc.LatestMsgSendTime = sendTime
	// log.Info("", "2 send message come here", *lc)
	_ = common.TriggerCmdUpdateConversation(ctx, common.UpdateConNode{ConID: lc.ConversationID, Action: constant.AddConOrUpLatMsg, Args: *lc}, c.GetCh())
}

func (c *Conversation) fileName(ftype string, id string) string {
	return fmt.Sprintf("msg_%s_%s", ftype, id)
}

func (c *Conversation) checkID(ctx context.Context, s *sdk_struct.MsgStruct,
	recvID, groupID string, options map[string]bool) (*model_struct.LocalConversation, error) {
	if recvID == "" && groupID == "" {
		return nil, sdkerrs.ErrArgs
	}
	s.SendID = c.loginUserID
	s.SenderPlatformID = c.platformID
	lc := &model_struct.LocalConversation{LatestMsgSendTime: s.CreateTime}
	//根据单聊群聊类型组装消息和会话
	if recvID == "" {
		g, err := c.full.GetGroupInfoByGroupID(ctx, groupID)
		if err != nil {
			return nil, err
		}
		lc.ShowName = g.GroupName
		lc.FaceURL = g.FaceURL
		switch g.GroupType {
		case constant.NormalGroup:
			s.SessionType = constant.GroupChatType
			lc.ConversationType = constant.GroupChatType
			lc.ConversationID = c.getConversationIDBySessionType(groupID, constant.GroupChatType)
		case constant.SuperGroup, constant.WorkingGroup:
			s.SessionType = constant.SuperGroupChatType
			lc.ConversationID = c.getConversationIDBySessionType(groupID, constant.SuperGroupChatType)
			lc.ConversationType = constant.SuperGroupChatType
		}
		s.GroupID = groupID
		lc.GroupID = groupID
		gm, err := c.db.GetGroupMemberInfoByGroupIDUserID(ctx, groupID, c.loginUserID)
		if err == nil && gm != nil {
			if gm.Nickname != "" {
				s.SenderNickname = gm.Nickname
			}
		}
		var attachedInfo sdk_struct.AttachedInfoElem
		attachedInfo.GroupHasReadInfo.GroupMemberCount = g.MemberCount
		s.AttachedInfoElem = &attachedInfo
	} else {
		s.SessionType = constant.SingleChatType
		s.RecvID = recvID
		lc.ConversationID = utils.GetConversationIDByMsg(s)
		lc.UserID = recvID
		lc.ConversationType = constant.SingleChatType
		oldLc, err := c.db.GetConversation(ctx, lc.ConversationID)
		if err == nil && oldLc.IsPrivateChat {
			options[constant.IsNotPrivate] = false
			var attachedInfo sdk_struct.AttachedInfoElem
			attachedInfo.IsPrivateChat = true
			attachedInfo.BurnDuration = oldLc.BurnDuration
			s.AttachedInfoElem = &attachedInfo
		}
		if err != nil {
			t := time.Now()
			faceUrl, name, err := c.getUserNameAndFaceURL(ctx, recvID)
			log.ZDebug(ctx, "GetUserNameAndFaceURL", "cost time", time.Since(t))
			if err != nil {
				return nil, err
			}
			lc.FaceURL = faceUrl
			lc.ShowName = name
		}

	}
	return lc, nil
}
func (c *Conversation) getConversationIDBySessionType(sourceID string, sessionType int) string {
	switch sessionType {
	case constant.SingleChatType:
		l := []string{c.loginUserID, sourceID}
		sort.Strings(l)
		return "si_" + strings.Join(l, "_") // single chat
	case constant.GroupChatType:
		return "g_" + sourceID // group chat
	case constant.SuperGroupChatType:
		return "sg_" + sourceID // super group chat
	case constant.NotificationChatType:
		return "sn_" + sourceID + "_" + c.loginUserID // server notification chat
	}
	return ""
}
func (c *Conversation) GetConversationIDBySessionType(_ context.Context, sourceID string, sessionType int) string {
	return c.getConversationIDBySessionType(sourceID, sessionType)
}
func (c *Conversation) SendMessage(ctx context.Context, s *sdk_struct.MsgStruct, recvID, groupID string, p *sdkws.OfflinePushInfo, isOnlineOnly bool) (*sdk_struct.MsgStruct, error) {
	filepathExt := func(name ...string) string {
		for _, path := range name {
			if ext := filepath.Ext(path); ext != "" {
				return ext
			}
		}
		return ""
	}
	options := make(map[string]bool, 2)
	lc, err := c.checkID(ctx, s, recvID, groupID, options)
	if err != nil {
		return nil, err
	}
	callback, _ := ctx.Value("callback").(open_im_sdk_callback.SendMsgCallBack)
	log.ZDebug(ctx, "before insert message is", "message", *s)
	if !isOnlineOnly {
		oldMessage, err := c.db.GetMessage(ctx, lc.ConversationID, s.ClientMsgID)
		if err != nil {
			localMessage := c.msgStructToLocalChatLog(s)
			err := c.db.InsertMessage(ctx, lc.ConversationID, localMessage)
			if err != nil {
				return nil, err
			}
			err = c.db.InsertSendingMessage(ctx, &model_struct.LocalSendingMessages{
				ConversationID: lc.ConversationID,
				ClientMsgID:    localMessage.ClientMsgID,
			})
			if err != nil {
				return nil, err
			}
		} else {
			if oldMessage.Status != constant.MsgStatusSendFailed {
				return nil, sdkerrs.ErrMsgRepeated
			} else {
				s.Status = constant.MsgStatusSending
				err = c.db.InsertSendingMessage(ctx, &model_struct.LocalSendingMessages{
					ConversationID: lc.ConversationID,
					ClientMsgID:    s.ClientMsgID,
				})
				if err != nil {
					return nil, err
				}
			}
		}
		lc.LatestMsg = utils.StructToJsonString(s)
		log.ZDebug(ctx, "send message come here", "conversion", *lc)
		_ = common.TriggerCmdUpdateConversation(ctx, common.UpdateConNode{ConID: lc.ConversationID, Action: constant.AddConOrUpLatMsg, Args: *lc}, c.GetCh())
	}

	var delFile []string
	//media file handle
	switch s.ContentType {
	case constant.Picture:
		if s.Status == constant.MsgStatusSendSuccess {
			s.Content = utils.StructToJsonString(s.PictureElem)
			break
		}
		var sourcePath string
		if utils.FileExist(s.PictureElem.SourcePath) {
			sourcePath = s.PictureElem.SourcePath
			delFile = append(delFile, utils.FileTmpPath(s.PictureElem.SourcePath, c.DataDir))
		} else {
			sourcePath = utils.FileTmpPath(s.PictureElem.SourcePath, c.DataDir)
			delFile = append(delFile, sourcePath)
		}
		// log.Info("", "file", sourcePath, delFile)
		log.ZDebug(ctx, "send picture", "path", sourcePath)

		res, err := c.file.UploadFile(ctx, &file.UploadFileReq{
			ContentType: s.PictureElem.SourcePicture.Type,
			Filepath:    sourcePath,
			Uuid:        s.PictureElem.SourcePicture.UUID,
			Name:        c.fileName("picture", s.ClientMsgID) + filepathExt(s.PictureElem.SourcePicture.UUID, sourcePath),
			Cause:       "msg-picture",
		}, NewUploadFileCallback(ctx, callback.OnProgress, s, lc.ConversationID, c.db))
		if err != nil {
			c.updateMsgStatusAndTriggerConversation(ctx, s.ClientMsgID, "", s.CreateTime, constant.MsgStatusSendFailed, s, lc, isOnlineOnly)
			return nil, err
		}
		s.PictureElem.SourcePicture.Url = res.URL
		s.PictureElem.BigPicture = s.PictureElem.SourcePicture
		u, err := url.Parse(res.URL)
		if err == nil {
			snapshot := u.Query()
			snapshot.Set("type", "image")
			snapshot.Set("width", "640")
			snapshot.Set("height", "640")
			u.RawQuery = snapshot.Encode()
			s.PictureElem.SnapshotPicture = &sdk_struct.PictureBaseInfo{
				Width:  640,
				Height: 640,
				Url:    u.String(),
			}
		} else {
			log.ZError(ctx, "parse url failed", err, "url", res.URL, "err", err)
			s.PictureElem.SnapshotPicture = s.PictureElem.SourcePicture
		}
		s.Content = utils.StructToJsonString(s.PictureElem)
	case constant.Sound:
		if s.Status == constant.MsgStatusSendSuccess {
			s.Content = utils.StructToJsonString(s.SoundElem)
			break
		}
		var sourcePath string
		if utils.FileExist(s.SoundElem.SoundPath) {
			sourcePath = s.SoundElem.SoundPath
			delFile = append(delFile, utils.FileTmpPath(s.SoundElem.SoundPath, c.DataDir))
		} else {
			sourcePath = utils.FileTmpPath(s.SoundElem.SoundPath, c.DataDir)
			delFile = append(delFile, sourcePath)
		}
		// log.Info("", "file", sourcePath, delFile)

		res, err := c.file.UploadFile(ctx, &file.UploadFileReq{
			ContentType: s.SoundElem.SoundType,
			Filepath:    sourcePath,
			Uuid:        s.SoundElem.UUID,
			Name:        c.fileName("voice", s.ClientMsgID) + filepathExt(s.SoundElem.UUID, sourcePath),
			Cause:       "msg-voice",
		}, NewUploadFileCallback(ctx, callback.OnProgress, s, lc.ConversationID, c.db))
		if err != nil {
			c.updateMsgStatusAndTriggerConversation(ctx, s.ClientMsgID, "", s.CreateTime, constant.MsgStatusSendFailed, s, lc, isOnlineOnly)
			return nil, err
		}
		s.SoundElem.SourceURL = res.URL
		s.Content = utils.StructToJsonString(s.SoundElem)
	case constant.Video:
		if s.Status == constant.MsgStatusSendSuccess {
			s.Content = utils.StructToJsonString(s.VideoElem)
			break
		}
		var videoPath string
		var snapPath string
		if utils.FileExist(s.VideoElem.VideoPath) {
			videoPath = s.VideoElem.VideoPath
			snapPath = s.VideoElem.SnapshotPath
			delFile = append(delFile, utils.FileTmpPath(s.VideoElem.VideoPath, c.DataDir))
			delFile = append(delFile, utils.FileTmpPath(s.VideoElem.SnapshotPath, c.DataDir))
		} else {
			videoPath = utils.FileTmpPath(s.VideoElem.VideoPath, c.DataDir)
			snapPath = utils.FileTmpPath(s.VideoElem.SnapshotPath, c.DataDir)
			delFile = append(delFile, videoPath)
			delFile = append(delFile, snapPath)
		}
		log.ZDebug(ctx, "file", "videoPath", videoPath, "snapPath", snapPath, "delFile", delFile)

		var wg sync.WaitGroup
		wg.Add(2)
		var putErrs error
		go func() {
			defer wg.Done()
			snapRes, err := c.file.UploadFile(ctx, &file.UploadFileReq{
				ContentType: s.VideoElem.SnapshotType,
				Filepath:    snapPath,
				Uuid:        s.VideoElem.SnapshotUUID,
				Name:        c.fileName("videoSnapshot", s.ClientMsgID) + filepathExt(s.VideoElem.SnapshotUUID, snapPath),
				Cause:       "msg-video-snapshot",
			}, nil)
			if err != nil {
				log.ZWarn(ctx, "upload video snapshot failed", err)
				return
			}
			s.VideoElem.SnapshotURL = snapRes.URL
		}()

		go func() {
			defer wg.Done()
			res, err := c.file.UploadFile(ctx, &file.UploadFileReq{
				ContentType: content_type.GetType(s.VideoElem.VideoType, filepath.Ext(s.VideoElem.VideoPath)),
				Filepath:    videoPath,
				Uuid:        s.VideoElem.VideoUUID,
				Name:        c.fileName("video", s.ClientMsgID) + filepathExt(s.VideoElem.VideoUUID, videoPath),
				Cause:       "msg-video",
			}, NewUploadFileCallback(ctx, callback.OnProgress, s, lc.ConversationID, c.db))
			if err != nil {
				c.updateMsgStatusAndTriggerConversation(ctx, s.ClientMsgID, "", s.CreateTime, constant.MsgStatusSendFailed, s, lc, isOnlineOnly)
				putErrs = err
			}
			if res != nil {
				s.VideoElem.VideoURL = res.URL
			}
		}()
		wg.Wait()
		if err := putErrs; err != nil {
			return nil, err
		}
		s.Content = utils.StructToJsonString(s.VideoElem)
	case constant.File:
		if s.Status == constant.MsgStatusSendSuccess {
			s.Content = utils.StructToJsonString(s.FileElem)
			break
		}
		name := s.FileElem.FileName
		if name == "" {
			name = s.FileElem.FilePath
		}
		if name == "" {
			name = fmt.Sprintf("msg_file_%s.unknown", s.ClientMsgID)
		}
		res, err := c.file.UploadFile(ctx, &file.UploadFileReq{
			ContentType: content_type.GetType(s.FileElem.FileType, filepath.Ext(s.FileElem.FilePath), filepath.Ext(s.FileElem.FileName)),
			Filepath:    s.FileElem.FilePath,
			Uuid:        s.FileElem.UUID,
			Name:        c.fileName("file", s.ClientMsgID) + "/" + filepath.Base(name),
			Cause:       "msg-file",
		}, NewUploadFileCallback(ctx, callback.OnProgress, s, lc.ConversationID, c.db))
		if err != nil {
			c.updateMsgStatusAndTriggerConversation(ctx, s.ClientMsgID, "", s.CreateTime, constant.MsgStatusSendFailed, s, lc, isOnlineOnly)
			return nil, err
		}
		s.FileElem.SourceURL = res.URL
		s.Content = utils.StructToJsonString(s.FileElem)
	case constant.Text:
		s.Content = utils.StructToJsonString(s.TextElem)
	case constant.AtText:
		s.Content = utils.StructToJsonString(s.AtTextElem)
	case constant.Location:
		s.Content = utils.StructToJsonString(s.LocationElem)
	case constant.Custom:
		s.Content = utils.StructToJsonString(s.CustomElem)
	case constant.Merger:
		s.Content = utils.StructToJsonString(s.MergeElem)
	case constant.Quote:
		s.Content = utils.StructToJsonString(s.QuoteElem)
	case constant.Card:
		s.Content = utils.StructToJsonString(s.CardElem)
	case constant.Face:
		s.Content = utils.StructToJsonString(s.FaceElem)
	case constant.AdvancedText:
		s.Content = utils.StructToJsonString(s.AdvancedTextElem)
	default:
		return nil, sdkerrs.ErrMsgContentTypeNotSupport
	}
	if utils.IsContainInt(int(s.ContentType), []int{constant.Picture, constant.Sound, constant.Video, constant.File}) {
		if !isOnlineOnly {
			localMessage := c.msgStructToLocalChatLog(s)
			log.ZDebug(ctx, "update message is ", "localMessage", localMessage)
			err = c.db.UpdateMessage(ctx, lc.ConversationID, localMessage)
			if err != nil {
				return nil, err
			}
		}
	}

	return c.sendMessageToServer(ctx, s, lc, callback, delFile, p, options, isOnlineOnly)

}
func (c *Conversation) SendMessageNotOss(ctx context.Context, s *sdk_struct.MsgStruct, recvID, groupID string,
	p *sdkws.OfflinePushInfo, isOnlineOnly bool) (*sdk_struct.MsgStruct, error) {
	options := make(map[string]bool, 2)
	lc, err := c.checkID(ctx, s, recvID, groupID, options)
	if err != nil {
		return nil, err
	}
	callback, _ := ctx.Value("callback").(open_im_sdk_callback.SendMsgCallBack)
	if !isOnlineOnly {
		oldMessage, err := c.db.GetMessage(ctx, lc.ConversationID, s.ClientMsgID)
		if err != nil {
			localMessage := c.msgStructToLocalChatLog(s)
			err := c.db.InsertMessage(ctx, lc.ConversationID, localMessage)
			if err != nil {
				return nil, err
			}
			err = c.db.InsertSendingMessage(ctx, &model_struct.LocalSendingMessages{
				ConversationID: lc.ConversationID,
				ClientMsgID:    localMessage.ClientMsgID,
			})
			if err != nil {
				return nil, err
			}
		} else {
			if oldMessage.Status != constant.MsgStatusSendFailed {
				return nil, sdkerrs.ErrMsgRepeated
			} else {
				s.Status = constant.MsgStatusSending
				err = c.db.InsertSendingMessage(ctx, &model_struct.LocalSendingMessages{
					ConversationID: lc.ConversationID,
					ClientMsgID:    s.ClientMsgID,
				})
				if err != nil {
					return nil, err
				}
			}
		}
	}
	lc.LatestMsg = utils.StructToJsonString(s)
	var delFile []string
	switch s.ContentType {
	case constant.Picture:
		s.Content = utils.StructToJsonString(s.PictureElem)
	case constant.Sound:
		s.Content = utils.StructToJsonString(s.SoundElem)
	case constant.Video:
		s.Content = utils.StructToJsonString(s.VideoElem)
	case constant.File:
		s.Content = utils.StructToJsonString(s.FileElem)
	case constant.Text:
		s.Content = utils.StructToJsonString(s.TextElem)
	case constant.AtText:
		s.Content = utils.StructToJsonString(s.AtTextElem)
	case constant.Location:
		s.Content = utils.StructToJsonString(s.LocationElem)
	case constant.Custom:
		s.Content = utils.StructToJsonString(s.CustomElem)
	case constant.Merger:
		s.Content = utils.StructToJsonString(s.MergeElem)
	case constant.Quote:
		s.Content = utils.StructToJsonString(s.QuoteElem)
	case constant.Card:
		s.Content = utils.StructToJsonString(s.CardElem)
	case constant.Face:
		s.Content = utils.StructToJsonString(s.FaceElem)
	case constant.AdvancedText:
		s.Content = utils.StructToJsonString(s.AdvancedTextElem)
	default:
		return nil, sdkerrs.ErrMsgContentTypeNotSupport
	}
	if utils.IsContainInt(int(s.ContentType), []int{constant.Picture, constant.Sound, constant.Video, constant.File}) {
		if isOnlineOnly {
			localMessage := c.msgStructToLocalChatLog(s)
			err = c.db.UpdateMessage(ctx, lc.ConversationID, localMessage)
			if err != nil {
				return nil, err
			}
		}
	}
	return c.sendMessageToServer(ctx, s, lc, callback, delFile, p, options, isOnlineOnly)
}

func (c *Conversation) sendMessageToServer(ctx context.Context, s *sdk_struct.MsgStruct, lc *model_struct.LocalConversation, callback open_im_sdk_callback.SendMsgCallBack,
	delFile []string, offlinePushInfo *sdkws.OfflinePushInfo, options map[string]bool, isOnlineOnly bool) (*sdk_struct.MsgStruct, error) {
	if isOnlineOnly {
		utils.SetSwitchFromOptions(options, constant.IsHistory, false)
		utils.SetSwitchFromOptions(options, constant.IsPersistent, false)
		utils.SetSwitchFromOptions(options, constant.IsSenderSync, false)
		utils.SetSwitchFromOptions(options, constant.IsConversationUpdate, false)
		utils.SetSwitchFromOptions(options, constant.IsSenderConversationUpdate, false)
		utils.SetSwitchFromOptions(options, constant.IsUnreadCount, false)
		utils.SetSwitchFromOptions(options, constant.IsOfflinePush, false)
	}
	//Protocol conversion
	var wsMsgData sdkws.MsgData
	copier.Copy(&wsMsgData, s)
	wsMsgData.AttachedInfo = utils.StructToJsonString(s.AttachedInfoElem)
	wsMsgData.Content = []byte(s.Content)
	wsMsgData.CreateTime = s.CreateTime
	wsMsgData.SendTime = 0
	wsMsgData.Options = options
	if wsMsgData.ContentType == constant.AtText {
		wsMsgData.AtUserIDList = s.AtTextElem.AtUserList
	}
	wsMsgData.OfflinePushInfo = offlinePushInfo
	s.Content = ""
	var sendMsgResp sdkws.UserSendMsgResp

	err := c.LongConnMgr.SendReqWaitResp(ctx, &wsMsgData, constant.SendMsg, &sendMsgResp)
	if err != nil {
		//if send message network timeout need to double-check message has received by db.
		if sdkerrs.ErrNetworkTimeOut.Is(err) && !isOnlineOnly {
			oldMessage, _ := c.db.GetMessage(ctx, lc.ConversationID, s.ClientMsgID)
			if oldMessage.Status == constant.MsgStatusSendSuccess {
				sendMsgResp.SendTime = oldMessage.SendTime
				sendMsgResp.ClientMsgID = oldMessage.ClientMsgID
				sendMsgResp.ServerMsgID = oldMessage.ServerMsgID
			} else {
				log.ZError(ctx, "send msg to server failed", err, "message", s)
				c.updateMsgStatusAndTriggerConversation(ctx, s.ClientMsgID, "", s.CreateTime,
					constant.MsgStatusSendFailed, s, lc, isOnlineOnly)
				return s, err
			}
		} else {
			log.ZError(ctx, "send msg to server failed", err, "message", s)
			c.updateMsgStatusAndTriggerConversation(ctx, s.ClientMsgID, "", s.CreateTime,
				constant.MsgStatusSendFailed, s, lc, isOnlineOnly)
			return s, err
		}
	}
	s.SendTime = sendMsgResp.SendTime
	s.Status = constant.MsgStatusSendSuccess
	s.ServerMsgID = sendMsgResp.ServerMsgID
	go func() {
		//remove media cache file
		for _, v := range delFile {
			err := os.Remove(v)
			if err != nil {
				// log.Error("", "remove failed,", err.Error(), v)
			}
			// log.Debug("", "remove file: ", v)
		}
		c.updateMsgStatusAndTriggerConversation(ctx, sendMsgResp.ClientMsgID, sendMsgResp.ServerMsgID, sendMsgResp.SendTime, constant.MsgStatusSendSuccess, s, lc, isOnlineOnly)
	}()
	return s, nil

}

func (c *Conversation) FindMessageList(ctx context.Context, req []*sdk_params_callback.ConversationArgs) (*sdk_params_callback.FindMessageListCallback, error) {
	var r sdk_params_callback.FindMessageListCallback
	type tempConversationAndMessageList struct {
		conversation *model_struct.LocalConversation
		msgIDList    []string
	}
	var s []*tempConversationAndMessageList
	for _, conversationsArgs := range req {
		localConversation, err := c.db.GetConversation(ctx, conversationsArgs.ConversationID)
		if err != nil {
			log.ZError(ctx, "GetConversation err", err, "conversationsArgs", conversationsArgs)
		} else {
			t := new(tempConversationAndMessageList)
			t.conversation = localConversation
			t.msgIDList = conversationsArgs.ClientMsgIDList
			s = append(s, t)
		}
	}
	for _, v := range s {
		messages, err := c.db.GetMessagesByClientMsgIDs(ctx, v.conversation.ConversationID, v.msgIDList)
		if err == nil {
			var tempMessageList []*sdk_struct.MsgStruct
			for _, message := range messages {
				temp := sdk_struct.MsgStruct{}
				temp.ClientMsgID = message.ClientMsgID
				temp.ServerMsgID = message.ServerMsgID
				temp.CreateTime = message.CreateTime
				temp.SendTime = message.SendTime
				temp.SessionType = message.SessionType
				temp.SendID = message.SendID
				temp.RecvID = message.RecvID
				temp.MsgFrom = message.MsgFrom
				temp.ContentType = message.ContentType
				temp.SenderPlatformID = message.SenderPlatformID
				temp.SenderNickname = message.SenderNickname
				temp.SenderFaceURL = message.SenderFaceURL
				temp.Content = message.Content
				temp.Seq = message.Seq
				temp.IsRead = message.IsRead
				temp.Status = message.Status
				temp.AttachedInfo = message.AttachedInfo
				temp.Ex = message.Ex
				temp.LocalEx = message.LocalEx
				err := c.msgHandleByContentType(&temp)
				if err != nil {
					log.ZError(ctx, "msgHandleByContentType err", err, "message", temp)
					continue
				}
				switch message.SessionType {
				case constant.GroupChatType:
					fallthrough
				case constant.SuperGroupChatType:
					temp.GroupID = temp.RecvID
					temp.RecvID = c.loginUserID
				}
				tempMessageList = append(tempMessageList, &temp)
			}
			findResultItem := sdk_params_callback.SearchByConversationResult{}
			findResultItem.ConversationID = v.conversation.ConversationID
			findResultItem.FaceURL = v.conversation.FaceURL
			findResultItem.ShowName = v.conversation.ShowName
			findResultItem.ConversationType = v.conversation.ConversationType
			findResultItem.MessageList = tempMessageList
			findResultItem.MessageCount = len(findResultItem.MessageList)
			r.FindResultItems = append(r.FindResultItems, &findResultItem)
			r.TotalCount += findResultItem.MessageCount
		} else {
			log.ZError(ctx, "GetMessagesByClientMsgIDs err", err, "conversationID", v.conversation.ConversationID, "msgIDList", v.msgIDList)
		}
	}
	return &r, nil

}

func (c *Conversation) GetAdvancedHistoryMessageList(ctx context.Context, req sdk_params_callback.GetAdvancedHistoryMessageListParams) (*sdk_params_callback.GetAdvancedHistoryMessageListCallback, error) {
	result, err := c.getAdvancedHistoryMessageList(ctx, req, false)
	if err != nil {
		return nil, err
	}
	if len(result.MessageList) == 0 {
		s := make([]*sdk_struct.MsgStruct, 0)
		result.MessageList = s
	}
	return result, nil
}

func (c *Conversation) GetAdvancedHistoryMessageListReverse(ctx context.Context, req sdk_params_callback.GetAdvancedHistoryMessageListParams) (*sdk_params_callback.GetAdvancedHistoryMessageListCallback, error) {
	result, err := c.getAdvancedHistoryMessageList(ctx, req, true)
	if err != nil {
		return nil, err
	}
	if len(result.MessageList) == 0 {
		s := make([]*sdk_struct.MsgStruct, 0)
		result.MessageList = s
	}
	return result, nil
}

func (c *Conversation) RevokeMessage(ctx context.Context, conversationID, clientMsgID string) error {
	return c.revokeOneMessage(ctx, conversationID, clientMsgID)
}

func (c *Conversation) TypingStatusUpdate(ctx context.Context, recvID, msgTip string) error {
	return c.typingStatusUpdate(ctx, recvID, msgTip)
}

// funcation (c *Conversation) MarkMessageAsReadByConID(ctx context.Context, conversationID string, msgIDList []string) error {
// 	if len(msgIDList) == 0 {
// 		_ = c.setOneConversationUnread(ctx, conversationID, 0)
// 		_ = common.TriggerCmdUpdateConversation(ctx, common.UpdateConNode{ConID: conversationID, Action: constant.UnreadCountSetZero}, c.GetCh())
// 		_ = common.TriggerCmdUpdateConversation(ctx, common.UpdateConNode{ConID: conversationID, Action: constant.ConChange, Args: []string{conversationID}}, c.GetCh())
// 		return nil
// 	}
// 	return nil

// }

// deprecated
// funcation (c *Conversation) MarkGroupMessageHasRead(ctx context.Context, groupID string) {
// 	conversationID := c.getConversationIDBySessionType(groupID, constant.GroupChatType)
// 	_ = c.setOneConversationUnread(ctx, conversationID, 0)
// 	_ = common.TriggerCmdUpdateConversation(ctx, common.UpdateConNode{ConID: conversationID, Action: constant.UnreadCountSetZero}, c.GetCh())
// 	_ = common.TriggerCmdUpdateConversation(ctx, common.UpdateConNode{ConID: conversationID, Action: constant.ConChange, Args: []string{conversationID}}, c.GetCh())
// }

// read draw
func (c *Conversation) MarkConversationMessageAsRead(ctx context.Context, conversationID string) error {
	return c.markConversationMessageAsRead(ctx, conversationID)
}

// deprecated
func (c *Conversation) MarkMessagesAsReadByMsgID(ctx context.Context, conversationID string, clientMsgIDs []string) error {
	return c.markMessagesAsReadByMsgID(ctx, conversationID, clientMsgIDs)
}

// delete
func (c *Conversation) DeleteMessageFromLocalStorage(ctx context.Context, conversationID string, clientMsgID string) error {
	return c.deleteMessageFromLocal(ctx, conversationID, clientMsgID)
}

func (c *Conversation) DeleteMessage(ctx context.Context, conversationID string, clientMsgID string) error {
	return c.deleteMessage(ctx, conversationID, clientMsgID)
}

func (c *Conversation) DeleteAllMsgFromLocalAndSvr(ctx context.Context) error {
	return c.deleteAllMsgFromLocalAndSvr(ctx)
}

func (c *Conversation) DeleteAllMessageFromLocalStorage(ctx context.Context) error {
	return c.deleteAllMsgFromLocal(ctx, true)
}

func (c *Conversation) ClearConversationAndDeleteAllMsg(ctx context.Context, conversationID string) error {
	return c.clearConversationFromLocalAndSvr(ctx, conversationID, c.db.ClearConversation)
}

func (c *Conversation) DeleteConversationAndDeleteAllMsg(ctx context.Context, conversationID string) error {
	return c.clearConversationFromLocalAndSvr(ctx, conversationID, c.db.ResetConversation)
}

// insert
func (c *Conversation) InsertSingleMessageToLocalStorage(ctx context.Context, s *sdk_struct.MsgStruct, recvID, sendID string) (*sdk_struct.MsgStruct, error) {
	if recvID == "" || sendID == "" {
		return nil, sdkerrs.ErrArgs
	}
	var conversation model_struct.LocalConversation
	if sendID != c.loginUserID {
		faceUrl, name, err := c.getUserNameAndFaceURL(ctx, sendID)
		if err != nil {
			//log.Error(operationID, "GetUserNameAndFaceURL err", err.Error(), sendID)
		}
		s.SenderFaceURL = faceUrl
		s.SenderNickname = name
		conversation.FaceURL = faceUrl
		conversation.ShowName = name
		conversation.UserID = sendID
		conversation.ConversationID = c.getConversationIDBySessionType(sendID, constant.SingleChatType)

	} else {
		conversation.UserID = recvID
		conversation.ConversationID = c.getConversationIDBySessionType(recvID, constant.SingleChatType)
		_, err := c.db.GetConversation(ctx, conversation.ConversationID)
		if err != nil {
			faceUrl, name, err := c.getUserNameAndFaceURL(ctx, recvID)
			if err != nil {
				return nil, err
			}
			conversation.FaceURL = faceUrl
			conversation.ShowName = name
		}
	}

	s.SendID = sendID
	s.RecvID = recvID
	s.ClientMsgID = utils.GetMsgID(s.SendID)
	s.SendTime = utils.GetCurrentTimestampByMill()
	s.SessionType = constant.SingleChatType
	s.Status = constant.MsgStatusSendSuccess
	localMessage := c.msgStructToLocalChatLog(s)
	conversation.LatestMsg = utils.StructToJsonString(s)
	conversation.ConversationType = constant.SingleChatType
	conversation.LatestMsgSendTime = s.SendTime
	err := c.insertMessageToLocalStorage(ctx, conversation.ConversationID, localMessage)
	if err != nil {
		return nil, err
	}
	_ = common.TriggerCmdUpdateConversation(ctx, common.UpdateConNode{ConID: conversation.ConversationID, Action: constant.AddConOrUpLatMsg, Args: conversation}, c.GetCh())
	return s, nil

}

func (c *Conversation) InsertGroupMessageToLocalStorage(ctx context.Context, s *sdk_struct.MsgStruct, groupID, sendID string) (*sdk_struct.MsgStruct, error) {
	if groupID == "" || sendID == "" {
		return nil, sdkerrs.ErrArgs
	}
	var conversation model_struct.LocalConversation
	var err error
	_, conversation.ConversationType, err = c.getConversationTypeByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}

	conversation.ConversationID = c.getConversationIDBySessionType(groupID, int(conversation.ConversationType))
	if sendID != c.loginUserID {
		faceUrl, name, err := c.getUserNameAndFaceURL(ctx, sendID)
		if err != nil {
			// log.Error("", "getUserNameAndFaceUrlByUid err", err.Error(), sendID)
		}
		s.SenderFaceURL = faceUrl
		s.SenderNickname = name
	}
	s.SendID = sendID
	s.RecvID = groupID
	s.GroupID = groupID
	s.ClientMsgID = utils.GetMsgID(s.SendID)
	s.SendTime = utils.GetCurrentTimestampByMill()
	s.SessionType = conversation.ConversationType
	s.Status = constant.MsgStatusSendSuccess
	localMessage := c.msgStructToLocalChatLog(s)
	conversation.LatestMsg = utils.StructToJsonString(s)
	conversation.LatestMsgSendTime = s.SendTime
	conversation.FaceURL = s.SenderFaceURL
	conversation.ShowName = s.SenderNickname
	err = c.insertMessageToLocalStorage(ctx, conversation.ConversationID, localMessage)
	if err != nil {
		return nil, err
	}
	_ = common.TriggerCmdUpdateConversation(ctx, common.UpdateConNode{ConID: conversation.ConversationID, Action: constant.AddConOrUpLatMsg, Args: conversation}, c.GetCh())
	return s, nil

}

func (c *Conversation) SearchLocalMessages(ctx context.Context, searchParam *sdk_params_callback.SearchLocalMessagesParams) (*sdk_params_callback.SearchLocalMessagesCallback, error) {
	searchParam.KeywordList = utils.TrimStringList(searchParam.KeywordList)
	return c.searchLocalMessages(ctx, searchParam)

}
func (c *Conversation) SetMessageLocalEx(ctx context.Context, conversationID string, clientMsgID string, localEx string) error {
	return c.db.UpdateColumnsMessage(ctx, conversationID, clientMsgID, map[string]interface{}{"local_ex": localEx})
}

func (c *Conversation) initBasicInfo(ctx context.Context, message *sdk_struct.MsgStruct, msgFrom, contentType int32) error {
	message.CreateTime = utils.GetCurrentTimestampByMill()
	message.SendTime = message.CreateTime
	message.IsRead = false
	message.Status = constant.MsgStatusSending
	message.SendID = c.loginUserID
	userInfo, err := c.db.GetLoginUser(ctx, c.loginUserID)
	if err != nil {
		return err
	} else {
		message.SenderFaceURL = userInfo.FaceURL
		message.SenderNickname = userInfo.Nickname
	}
	ClientMsgID := utils.GetMsgID(message.SendID)
	message.ClientMsgID = ClientMsgID
	message.MsgFrom = msgFrom
	message.ContentType = contentType
	message.SenderPlatformID = c.platformID
	message.IsExternalExtensions = c.IsExternalExtensions
	return nil
}

//// 删除本地和服务器
//// 删除本地的话不用改服务器的数据
//// 删除服务器的话，需要把本地的消息状态改成删除
//funcation (c *Conversation) DeleteConversationFromLocalAndSvr(ctx context.Context, conversationID string) error {
//	// Use conversationID to remove conversations and messages from the server first
//	err := c.clearConversationFromSvr(ctx, conversationID)
//	if err != nil {
//		return err
//	}
//	return c.deleteConversation(ctx, conversationID)
//}

func (c *Conversation) getConversationTypeByGroupID(ctx context.Context, groupID string) (conversationID string, conversationType int32, err error) {
	g, err := c.full.GetGroupInfoByGroupID(ctx, groupID)
	if err != nil {
		return "", 0, utils.Wrap(err, "get group info error")
	}
	switch g.GroupType {
	case constant.NormalGroup:
		return c.getConversationIDBySessionType(groupID, constant.GroupChatType), constant.GroupChatType, nil
	case constant.SuperGroup, constant.WorkingGroup:
		return c.getConversationIDBySessionType(groupID, constant.SuperGroupChatType), constant.SuperGroupChatType, nil
	default:
		return "", 0, sdkerrs.ErrGroupType
	}
}

func (c *Conversation) SetMessageReactionExtensions(ctx context.Context, s *sdk_struct.MsgStruct, req []*server_api_params.KeyValue) ([]*server_api_params.ExtensionResult, error) {
	return c.setMessageReactionExtensions(ctx, s, req)

}

func (c *Conversation) AddMessageReactionExtensions(ctx context.Context, s *sdk_struct.MsgStruct, reactionExtensionList []*server_api_params.KeyValue) ([]*server_api_params.ExtensionResult, error) {
	return c.addMessageReactionExtensions(ctx, s, reactionExtensionList)

}

func (c *Conversation) DeleteMessageReactionExtensions(ctx context.Context, s *sdk_struct.MsgStruct, reactionExtensionKeyList []string) ([]*server_api_params.ExtensionResult, error) {
	return c.deleteMessageReactionExtensions(ctx, s, reactionExtensionKeyList)
}

func (c *Conversation) GetMessageListReactionExtensions(ctx context.Context, conversationID string, messageList []*sdk_struct.MsgStruct) ([]*server_api_params.SingleMessageExtensionResult, error) {
	return c.getMessageListReactionExtensions(ctx, conversationID, messageList)

}
func (c *Conversation) SearchConversation(ctx context.Context, searchParam string) ([]*server_api_params.Conversation, error) {
	// Check if search parameter is empty
	if searchParam == "" {
		return nil, sdkerrs.ErrArgs.Wrap("search parameter cannot be empty")
	}

	// Perform the search in your database or data source
	// This is a placeholder for the actual database call
	conversations, err := c.db.SearchConversations(ctx, searchParam)
	if err != nil {
		// Handle any errors that occurred during the search
		return nil, err
	}
	apiConversations := make([]*server_api_params.Conversation, len(conversations))
	for i, localConv := range conversations {
		// Create new server_api_params.Conversation and map fields from localConv
		apiConv := &server_api_params.Conversation{
			ConversationID:        localConv.ConversationID,
			ConversationType:      localConv.ConversationType,
			UserID:                localConv.UserID,
			GroupID:               localConv.GroupID,
			RecvMsgOpt:            localConv.RecvMsgOpt,
			UnreadCount:           localConv.UnreadCount,
			DraftTextTime:         localConv.DraftTextTime,
			IsPinned:              localConv.IsPinned,
			IsPrivateChat:         localConv.IsPrivateChat,
			BurnDuration:          localConv.BurnDuration,
			GroupAtType:           localConv.GroupAtType,
			IsNotInGroup:          localConv.IsNotInGroup,
			UpdateUnreadCountTime: localConv.UpdateUnreadCountTime,
			AttachedInfo:          localConv.AttachedInfo,
			Ex:                    localConv.Ex,
		}
		apiConversations[i] = apiConv
	}
	// Return the list of conversations
	return apiConversations, nil
}

/**
**Get some reaction extensions in reactionExtensionKeyList of message list
 */
//funcation (c *Conversation) GetMessageListSomeReactionExtensions(ctx context.Context, messageList, reactionExtensionKeyList, operationID string) {
//	var messagelist []*sdk_struct.MsgStruct
//	common.JsonUnmarshalAndArgsValidate(messageList, &messagelist, callback, operationID)
//	var list []string
//	common.JsonUnmarshalAndArgsValidate(reactionExtensionKeyList, &list, callback, operationID)
//	result := c.getMessageListSomeReactionExtensions(callback, messagelist, list, operationID)
//	callback.OnSuccess(utils.StructToJsonString(result))
//	log.NewInfo(operationID, utils.GetSelfFuncName(), "callback: ", utils.StructToJsonString(result))
//}
//funcation (c *Conversation) SetTypeKeyInfo(ctx context.Context, message, typeKey, ex string, isCanRepeat bool, operationID string) {
//	s := sdk_struct.MsgStruct{}
//	common.JsonUnmarshalAndArgsValidate(message, &s, callback, operationID)
//	result := c.setTypeKeyInfo(callback, &s, typeKey, ex, isCanRepeat, operationID)
//	callback.OnSuccess(utils.StructToJsonString(result))
//	log.NewInfo(operationID, utils.GetSelfFuncName(), "callback: ", utils.StructToJsonString(result))
//}
//funcation (c *Conversation) GetTypeKeyListInfo(ctx context.Context, message, typeKeyList, operationID string) {
//	s := sdk_struct.MsgStruct{}
//	common.JsonUnmarshalAndArgsValidate(message, &s, callback, operationID)
//	var list []string
//	common.JsonUnmarshalAndArgsValidate(typeKeyList, &list, callback, operationID)
//	result := c.getTypeKeyListInfo(callback, &s, list, operationID)
//	callback.OnSuccess(utils.StructToJsonString(result))
//	log.NewInfo(operationID, utils.GetSelfFuncName(), "callback: ", utils.StructToJsonString(result))
//}
//funcation (c *Conversation) GetAllTypeKeyInfo(ctx context.Context, message, operationID string) {
//	s := sdk_struct.MsgStruct{}
//	common.JsonUnmarshalAndArgsValidate(message, &s, callback, operationID)
//	result := c.getAllTypeKeyInfo(callback, &s, operationID)
//	callback.OnSuccess(utils.StructToJsonString(result))
//	log.NewInfo(operationID, utils.GetSelfFuncName(), "callback: ", utils.StructToJsonString(result))
//}
