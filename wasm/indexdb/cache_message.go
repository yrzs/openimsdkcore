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

//go:build js && wasm
// +build js,wasm

package indexdb

import (
	"context"
	"github.com/yrzs/openimsdkcore/wasm/exec"
)

import (
	"github.com/yrzs/openimsdkcore/pkg/db/model_struct"
	"github.com/yrzs/openimsdkcore/pkg/utils"
)

type LocalCacheMessage struct {
}

func NewLocalCacheMessage() *LocalCacheMessage {
	return &LocalCacheMessage{}
}

func (i *LocalCacheMessage) BatchInsertTempCacheMessageList(ctx context.Context, MessageList []*model_struct.TempCacheLocalChatLog) error {
	_, err := exec.Exec(utils.StructToJsonString(MessageList))
	return err
}

func (i *LocalCacheMessage) InsertTempCacheMessage(ctx context.Context, Message *model_struct.TempCacheLocalChatLog) error {
	_, err := exec.Exec(utils.StructToJsonString(Message))
	return err
}
