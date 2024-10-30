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
	"github.com/yrzs/openimsdkcore/internal/util"
	"github.com/yrzs/openimsdkcore/pkg/constant"
	"github.com/yrzs/openimsdkcore/pkg/db/model_struct"
	pbUser "github.com/yrzs/openimsdkprotocol/user"
	userPb "github.com/yrzs/openimsdkprotocol/user"

	"github.com/yrzs/openimsdkprotocol/sdkws"
)

func (u *User) GetUsersInfo(ctx context.Context, userIDs []string) ([]*model_struct.LocalUser, error) {
	return u.GetUsersInfoFromSvr(ctx, userIDs)
}

func (u *User) GetSelfUserInfo(ctx context.Context) (*model_struct.LocalUser, error) {
	return u.getSelfUserInfo(ctx)
}

func (u *User) SetSelfInfo(ctx context.Context, userInfo *sdkws.UserInfoWithEx) error {
	return u.updateSelfUserInfo(ctx, userInfo)
}

func (u *User) SetGlobalRecvMessageOpt(ctx context.Context, opt int) error {
	if err := util.ApiPost(ctx, constant.SetGlobalRecvMessageOptRouter,
		&pbUser.SetGlobalRecvMessageOptReq{UserID: u.loginUserID, GlobalRecvMsgOpt: int32(opt)}, nil); err != nil {
		return err
	}
	u.SyncLoginUserInfo(ctx)
	return nil
}

func (u *User) UpdateMsgSenderInfo(ctx context.Context, nickname, faceURL string) (err error) {
	if nickname != "" {
		if err = u.DataBase.UpdateMsgSenderNickname(ctx, u.loginUserID, nickname, constant.SingleChatType); err != nil {
			return err
		}
	}
	if faceURL != "" {
		if err = u.DataBase.UpdateMsgSenderFaceURL(ctx, u.loginUserID, faceURL, constant.SingleChatType); err != nil {
			return err
		}
	}
	return nil
}

func (u *User) SubscribeUsersStatus(ctx context.Context, userIDs []string) ([]*userPb.OnlineStatus, error) {
	userStatus, err := u.subscribeUsersStatus(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	u.OnlineStatusCache.DeleteAll()
	u.OnlineStatusCache.StoreAll(func(value *userPb.OnlineStatus) string {
		return value.UserID
	}, userStatus)
	return userStatus, nil
}

func (u *User) UnsubscribeUsersStatus(ctx context.Context, userIDs []string) error {
	u.OnlineStatusCache.DeleteAll()
	return u.unsubscribeUsersStatus(ctx, userIDs)
}

func (u *User) GetSubscribeUsersStatus(ctx context.Context) ([]*userPb.OnlineStatus, error) {
	return u.getSubscribeUsersStatus(ctx)
}

func (u *User) GetUserStatus(ctx context.Context, userIDs []string) ([]*userPb.OnlineStatus, error) {
	return u.getUserStatus(ctx, userIDs)
}
