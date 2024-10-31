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

package user

import (
	"context"
	"fmt"
	"github.com/yrzs/openimsdkcore/internal/cache"
	"github.com/yrzs/openimsdkcore/internal/util"
	"github.com/yrzs/openimsdkcore/pkg/db/db_interface"
	"github.com/yrzs/openimsdkcore/pkg/db/model_struct"
	"github.com/yrzs/openimsdkcore/pkg/sdkerrs"
	"github.com/yrzs/openimsdkcore/pkg/syncer"

	authPb "github.com/openimsdk/protocol/auth"
	"github.com/openimsdk/protocol/sdkws"
	userPb "github.com/openimsdk/protocol/user"
	"github.com/yrzs/openimsdktools/log"

	PbConstant "github.com/openimsdk/protocol/constant"
	"github.com/yrzs/openimsdkcore/open_im_sdk_callback"
	"github.com/yrzs/openimsdkcore/pkg/common"
	"github.com/yrzs/openimsdkcore/pkg/constant"
	"github.com/yrzs/openimsdkcore/pkg/utils"
)

type BasicInfo struct {
	Nickname string
	FaceURL  string
}

// User is a struct that represents a user in the system.
type User struct {
	db_interface.DataBase
	loginUserID       string
	listener          func() open_im_sdk_callback.OnUserListener
	userSyncer        *syncer.Syncer[*model_struct.LocalUser, string]
	conversationCh    chan common.Cmd2Value
	UserBasicCache    *cache.Cache[string, *BasicInfo]
	OnlineStatusCache *cache.Cache[string, *userPb.OnlineStatus]
}

// SetListener sets the user's listener.
func (u *User) SetListener(listener func() open_im_sdk_callback.OnUserListener) {
	u.listener = listener
}

// NewUser creates a new User object.
func NewUser(dataBase db_interface.DataBase, loginUserID string, conversationCh chan common.Cmd2Value) *User {
	user := &User{DataBase: dataBase, loginUserID: loginUserID, conversationCh: conversationCh}
	user.initSyncer()
	user.UserBasicCache = cache.NewCache[string, *BasicInfo]()
	user.OnlineStatusCache = cache.NewCache[string, *userPb.OnlineStatus]()
	return user
}

func (u *User) initSyncer() {
	u.userSyncer = syncer.New(
		func(ctx context.Context, value *model_struct.LocalUser) error {
			return u.InsertLoginUser(ctx, value)
		},
		func(ctx context.Context, value *model_struct.LocalUser) error {
			return fmt.Errorf("not support delete user %s", value.UserID)
		},
		func(ctx context.Context, serverUser, localUser *model_struct.LocalUser) error {
			return u.DataBase.UpdateLoginUser(context.Background(), serverUser)
		},
		func(user *model_struct.LocalUser) string {
			return user.UserID
		},
		nil,
		func(ctx context.Context, state int, server, local *model_struct.LocalUser) error {
			switch state {
			case syncer.Update:
				u.listener().OnSelfInfoUpdated(utils.StructToJsonString(server))
				if server.Nickname != local.Nickname || server.FaceURL != local.FaceURL {
					_ = common.TriggerCmdUpdateMessage(ctx, common.UpdateMessageNode{Action: constant.UpdateMsgFaceUrlAndNickName,
						Args: common.UpdateMessageInfo{SessionType: constant.SingleChatType, UserID: server.UserID, FaceURL: server.FaceURL, Nickname: server.Nickname}}, u.conversationCh)
				}
			}
			return nil
		},
	)
}

//func (u *User) equal(a, b *model_struct.LocalUser) bool {
//	if a.CreateTime != b.CreateTime {
//		log.ZDebug(context.Background(), "user equal", "a", a.CreateTime, "b", b.CreateTime)
//	}
//	if a.UserID != b.UserID {
//		log.ZDebug(context.Background(), "user equal", "a", a.UserID, "b", b.UserID)
//	}
//	if a.Ex != b.Ex {
//		log.ZDebug(context.Background(), "user equal", "a", a.Ex, "b", b.Ex)
//	}
//
//	if a.Nickname != b.Nickname {
//		log.ZDebug(context.Background(), "user equal", "a", a.Nickname, "b", b.Nickname)
//	}
//	if a.FaceURL != b.FaceURL {
//		log.ZDebug(context.Background(), "user equal", "a", a.FaceURL, "b", b.FaceURL)
//	}
//	if a.AttachedInfo != b.AttachedInfo {
//		log.ZDebug(context.Background(), "user equal", "a", a.AttachedInfo, "b", b.AttachedInfo)
//	}
//	if a.GlobalRecvMsgOpt != b.GlobalRecvMsgOpt {
//		log.ZDebug(context.Background(), "user equal", "a", a.GlobalRecvMsgOpt, "b", b.GlobalRecvMsgOpt)
//	}
//	if a.AppMangerLevel != b.AppMangerLevel {
//		log.ZDebug(context.Background(), "user equal", "a", a.AppMangerLevel, "b", b.AppMangerLevel)
//	}
//	return a.UserID == b.UserID && a.Nickname == b.Nickname && a.FaceURL == b.FaceURL &&
//		a.CreateTime == b.CreateTime && a.AttachedInfo == b.AttachedInfo &&
//		a.Ex == b.Ex && a.GlobalRecvMsgOpt == b.GlobalRecvMsgOpt && a.AppMangerLevel == b.AppMangerLevel
//}

// DoNotification handles incoming notifications for the user.
func (u *User) DoNotification(ctx context.Context, msg *sdkws.MsgData) {
	log.ZDebug(ctx, "user notification", "msg", *msg)
	go func() {
		switch msg.ContentType {
		case constant.UserInfoUpdatedNotification:
			u.userInfoUpdatedNotification(ctx, msg)
		case constant.UserStatusChangeNotification:
			u.userStatusChangeNotification(ctx, msg)
		default:
			// log.Error(operationID, "type failed ", msg.ClientMsgID, msg.ServerMsgID, msg.ContentType)
		}
	}()
}

// userInfoUpdatedNotification handles notifications about updated user information.
func (u *User) userInfoUpdatedNotification(ctx context.Context, msg *sdkws.MsgData) {
	log.ZDebug(ctx, "userInfoUpdatedNotification", "msg", *msg)
	tips := sdkws.UserInfoUpdatedTips{}
	if err := utils.UnmarshalNotificationElem(msg.Content, &tips); err != nil {
		log.ZError(ctx, "comm.UnmarshalTips failed", err, "msg", msg.Content)
		return
	}

	if tips.UserID == u.loginUserID {
		u.SyncLoginUserInfo(ctx)
	} else {
		log.ZDebug(ctx, "detail.UserID != u.loginUserID, do nothing", "detail.UserID", tips.UserID, "u.loginUserID", u.loginUserID)
	}
}

// userStatusChangeNotification get subscriber status change callback
func (u *User) userStatusChangeNotification(ctx context.Context, msg *sdkws.MsgData) {
	log.ZDebug(ctx, "userStatusChangeNotification", "msg", *msg)
	tips := sdkws.UserStatusChangeTips{}
	if err := utils.UnmarshalNotificationElem(msg.Content, &tips); err != nil {
		log.ZError(ctx, "comm.UnmarshalTips failed", err, "msg", msg.Content)
		return
	}
	if tips.FromUserID == u.loginUserID {
		log.ZDebug(ctx, "self terminal login", "tips", tips)
		return
	}
	u.SyncUserStatus(ctx, tips.FromUserID, tips.Status, tips.PlatformID)
}

// GetUsersInfoFromSvr retrieves user information from the server.
func (u *User) GetUsersInfoFromSvr(ctx context.Context, userIDs []string) ([]*model_struct.LocalUser, error) {
	resp, err := util.CallApi[userPb.GetDesignateUsersResp](ctx, constant.GetUsersInfoRouter, userPb.GetDesignateUsersReq{UserIDs: userIDs})
	if err != nil {
		return nil, sdkerrs.Warp(err, "GetUsersInfoFromSvr failed")
	}
	return util.Batch(ServerUserToLocalUser, resp.UsersInfo), nil
}

// GetSingleUserFromSvr retrieves user information from the server.
func (u *User) GetSingleUserFromSvr(ctx context.Context, userID string) (*model_struct.LocalUser, error) {
	users, err := u.GetUsersInfoFromSvr(ctx, []string{userID})
	if err != nil {
		return nil, err
	}
	if len(users) > 0 {
		return users[0], nil
	}
	return nil, sdkerrs.ErrUserIDNotFound.Wrap(fmt.Sprintf("getSelfUserInfo failed, userID: %s not exist", userID))
}

// getSelfUserInfo retrieves the user's information.
func (u *User) getSelfUserInfo(ctx context.Context) (*model_struct.LocalUser, error) {
	userInfo, errLocal := u.GetLoginUser(ctx, u.loginUserID)
	if errLocal != nil {
		srvUserInfo, errServer := u.GetServerUserInfo(ctx, []string{u.loginUserID})
		if errServer != nil {
			return nil, errServer
		}
		if len(srvUserInfo) == 0 {
			return nil, sdkerrs.ErrUserIDNotFound
		}
		userInfo = ServerUserToLocalUser(srvUserInfo[0])
		_ = u.InsertLoginUser(ctx, userInfo)
	}
	return userInfo, nil
}

// updateSelfUserInfo updates the user's information.
func (u *User) updateSelfUserInfo(ctx context.Context, userInfo *sdkws.UserInfoWithEx) error {
	userInfo.UserID = u.loginUserID
	if err := util.ApiPost(ctx, constant.UpdateSelfUserInfoExRouter, userPb.UpdateUserInfoExReq{UserInfo: userInfo}, nil); err != nil {
		return err
	}
	_ = u.SyncLoginUserInfo(ctx)
	return nil
}

// CRUD user command
//func (u *User) ProcessUserCommandAdd(ctx context.Context, userCommand *sdkws.ProcessUserCommand) error {
//	if err := util.ApiPost(ctx, constant.ProcessUserCommandAdd, userPb.ProcessUserCommandAddReq{UserID: u.loginUserID, Type: userCommand.Type, Uuid: userCommand.Uuid, Value: userCommand.Value}, nil); err != nil {
//		return err
//	}
//	return nil
//}
//func (u *User) ProcessUserCommandDelete(ctx context.Context, userCommand *sdkws.ProcessUserCommand) error {
//	if err := util.ApiPost(ctx, constant.ProcessUserCommandAdd, userPb.ProcessUserCommandDeleteReq{UserID: u.loginUserID, Type: userCommand.Type, Uuid: userCommand.Uuid, Value: userCommand.Value}, nil); err != nil {
//		return err
//	}
//	return nil
//}
//func (u *User) ProcessUserCommandUpdate(ctx context.Context, userCommand *sdkws.ProcessUserCommand) error {
//	if err := util.ApiPost(ctx, constant.ProcessUserCommandAdd, userPb.ProcessUserCommandUpdateReq{UserID: u.loginUserID, Type: userCommand.Type, Uuid: userCommand.Uuid, Value: userCommand.Value}, nil); err != nil {
//		return err
//	}
//	return nil
//}
//func (u *User) ProcessUserCommandGet(ctx context.Context, userCommand *sdkws.ProcessUserCommand) error {
//	if err := util.ApiPost(ctx, constant.ProcessUserCommandAdd, userPb.ProcessUserCommandGetReq{UserID: u.loginUserID, Type: userCommand.Type}, nil); err != nil {
//		return err
//	}
//	return nil
//}

// ParseTokenFromSvr parses a token from the server.
func (u *User) ParseTokenFromSvr(ctx context.Context) (int64, error) {
	resp, err := util.CallApi[authPb.ParseTokenResp](ctx, constant.ParseTokenRouter, authPb.ParseTokenReq{})
	return resp.ExpireTimeSeconds, err
}

// GetServerUserInfo retrieves user information from the server.
func (u *User) GetServerUserInfo(ctx context.Context, userIDs []string) ([]*sdkws.UserInfo, error) {
	resp, err := util.CallApi[userPb.GetDesignateUsersResp](ctx, constant.GetUsersInfoRouter, &userPb.GetDesignateUsersReq{UserIDs: userIDs})
	if err != nil {
		return nil, err
	}
	return resp.UsersInfo, nil
}

// subscribeUsersStatus Presence status of subscribed users.
func (u *User) subscribeUsersStatus(ctx context.Context, userIDs []string) ([]*userPb.OnlineStatus, error) {
	resp, err := util.CallApi[userPb.SubscribeOrCancelUsersStatusResp](ctx, constant.SubscribeUsersStatusRouter, &userPb.SubscribeOrCancelUsersStatusReq{
		UserID:  u.loginUserID,
		UserIDs: userIDs,
		Genre:   PbConstant.SubscriberUser,
	})
	if err != nil {
		return nil, err
	}
	return resp.StatusList, nil
}

// unsubscribeUsersStatus Unsubscribe a user's presence.
func (u *User) unsubscribeUsersStatus(ctx context.Context, userIDs []string) error {
	_, err := util.CallApi[userPb.SubscribeOrCancelUsersStatusResp](ctx, constant.SubscribeUsersStatusRouter, &userPb.SubscribeOrCancelUsersStatusReq{
		UserID:  u.loginUserID,
		UserIDs: userIDs,
		Genre:   PbConstant.Unsubscribe,
	})
	if err != nil {
		return err
	}
	return nil
}

// getSubscribeUsersStatus Get the online status of subscribers.
func (u *User) getSubscribeUsersStatus(ctx context.Context) ([]*userPb.OnlineStatus, error) {
	resp, err := util.CallApi[userPb.GetSubscribeUsersStatusResp](ctx, constant.GetSubscribeUsersStatusRouter, &userPb.GetSubscribeUsersStatusReq{
		UserID: u.loginUserID,
	})
	if err != nil {
		return nil, err
	}
	return resp.StatusList, nil
}

// getUserStatus Get the online status of users.
func (u *User) getUserStatus(ctx context.Context, userIDs []string) ([]*userPb.OnlineStatus, error) {
	resp, err := util.CallApi[userPb.GetUserStatusResp](ctx, constant.GetUserStatusRouter, &userPb.GetUserStatusReq{
		UserID:  u.loginUserID,
		UserIDs: userIDs,
	})
	if err != nil {
		return nil, err
	}
	return resp.StatusList, nil
}
