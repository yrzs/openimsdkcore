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

//go:build !js
// +build !js

package db

import (
	"context"
	"errors"
	"github.com/yrzs/openimsdkcore/pkg/constant"
	"github.com/yrzs/openimsdkcore/pkg/db/model_struct"
	"github.com/yrzs/openimsdkcore/pkg/utils"
)

func (d *DataBase) InsertSuperGroup(ctx context.Context, groupInfo *model_struct.LocalGroup) error {
	d.superGroupMtx.Lock()
	defer d.superGroupMtx.Unlock()
	return utils.Wrap(d.conn.WithContext(ctx).Table(constant.SuperGroupTableName).Create(groupInfo).Error, "InsertSuperGroup failed")
}

func (d *DataBase) DeleteAllSuperGroup(ctx context.Context) error {
	d.superGroupMtx.Lock()
	defer d.superGroupMtx.Unlock()
	return utils.Wrap(d.conn.WithContext(ctx).Table(constant.SuperGroupTableName).Delete(&model_struct.LocalGroup{}).Error, "DeleteAllSuperGroup failed")
}

func (d *DataBase) GetSuperGroupInfoByGroupID(ctx context.Context, groupID string) (*model_struct.LocalGroup, error) {
	d.superGroupMtx.Lock()
	defer d.superGroupMtx.Unlock()
	var g model_struct.LocalGroup
	return &g, utils.Wrap(d.conn.WithContext(ctx).Table(constant.SuperGroupTableName).Where("group_id = ?", groupID).Take(&g).Error, "GetGroupList failed")
}

func (d *DataBase) UpdateSuperGroup(ctx context.Context, groupInfo *model_struct.LocalGroup) error {
	d.superGroupMtx.Lock()
	defer d.superGroupMtx.Unlock()

	t := d.conn.WithContext(ctx).Table(constant.SuperGroupTableName).Select("*").Updates(*groupInfo)
	if t.RowsAffected == 0 {
		return utils.Wrap(errors.New("RowsAffected == 0"), "no update")
	}
	return utils.Wrap(t.Error, "")
}

func (d *DataBase) DeleteSuperGroup(ctx context.Context, groupID string) error {
	d.superGroupMtx.Lock()
	defer d.superGroupMtx.Unlock()
	localGroup := model_struct.LocalGroup{GroupID: groupID}
	return utils.Wrap(d.conn.WithContext(ctx).Table(constant.SuperGroupTableName).Delete(&localGroup).Error, "DeleteSuperGroup failed")
}
