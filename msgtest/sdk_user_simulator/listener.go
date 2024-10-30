package sdk_user_simulator

import (
	"github.com/yrzs/openimsdkcore/pkg/constant"
	"github.com/yrzs/openimsdkcore/pkg/utils"
	"github.com/yrzs/openimsdkcore/sdk_struct"
)

type conversationCallBack struct {
}

func (c *conversationCallBack) OnSyncServerFailed() {
}

func (c *conversationCallBack) OnNewConversation(conversationList string) {
}

func (c *conversationCallBack) OnConversationChanged(conversationList string) {
}

func (c *conversationCallBack) OnTotalUnreadMessageCountChanged(totalUnreadCount int32) {
}

func (c *conversationCallBack) OnRecvMessageExtensionsChanged(msgID string, reactionExtensionList string) {
}

func (c *conversationCallBack) OnRecvMessageExtensionsDeleted(msgID string, reactionExtensionKeyList string) {
}

func (c *conversationCallBack) OnSyncServerProgress(progress int) {
}

func (c *conversationCallBack) OnSyncServerStart() {

}

func (c *conversationCallBack) OnSyncServerFinish() {

}

func (c *conversationCallBack) OnConversationUserInputStatusChanged(change string) {

}

type userCallback struct {
}

func (c userCallback) OnUserStatusChanged(statusMap string) {

}

func (userCallback) OnSelfInfoUpdated(callbackData string) {

}

type SingleMessage struct {
	SendID      string
	ClientMsgID string
	Delay       int64
}
type MsgListenerCallBak struct {
	userID      string
	GroupDelay  map[string][]*SingleMessage
	SingleDelay map[string][]*SingleMessage
}

func NewMsgListenerCallBak(userID string) *MsgListenerCallBak {
	return &MsgListenerCallBak{userID: userID,
		GroupDelay:  make(map[string][]*SingleMessage),
		SingleDelay: make(map[string][]*SingleMessage)}
}

func (m *MsgListenerCallBak) OnRecvNewMessage(message string) {
	var sm sdk_struct.MsgStruct
	utils.JsonStringToStruct(message, &sm)
	switch sm.SessionType {
	case constant.SingleChatType:
		m.SingleDelay[sm.SendID] =
			append(m.SingleDelay[sm.SendID], &SingleMessage{SendID: sm.SendID, ClientMsgID: sm.ClientMsgID, Delay: GetRelativeServerTime() - sm.SendTime})
	case constant.SuperGroupChatType:
		m.GroupDelay[sm.GroupID] =
			append(m.GroupDelay[sm.GroupID], &SingleMessage{SendID: sm.SendID, ClientMsgID: sm.ClientMsgID, Delay: GetRelativeServerTime() - sm.SendTime})
	default:
	}

}

func (m *MsgListenerCallBak) OnRecvC2CReadReceipt(msgReceiptList string) {
}

func (m *MsgListenerCallBak) OnMsgDeleted(s string) {}

func (m *MsgListenerCallBak) OnRecvOfflineNewMessage(message string) {
}

func (m *MsgListenerCallBak) OnRecvMessageExtensionsAdded(msgID string, reactionExtensionList string) {

}

func (m *MsgListenerCallBak) OnRecvGroupReadReceipt(groupMsgReceiptList string) {
}
func (m *MsgListenerCallBak) OnNewRecvMessageRevoked(messageRevoked string) {
}

func (m *MsgListenerCallBak) OnRecvMessageExtensionsChanged(msgID string, reactionExtensionList string) {

}
func (m *MsgListenerCallBak) OnRecvMessageExtensionsDeleted(msgID string, reactionExtensionKeyList string) {
}

func (m *MsgListenerCallBak) OnRecvOnlineOnlyMessage(message string) {

}

type testFriendListener struct {
}

func (testFriendListener) OnFriendApplicationAdded(callbackInfo string) {

}
func (testFriendListener) OnFriendApplicationDeleted(callbackInfo string) {

}

func (testFriendListener) OnFriendApplicationAccepted(callbackInfo string) {

}

func (testFriendListener) OnFriendApplicationRejected(callbackInfo string) {

}

func (testFriendListener) OnFriendAdded(callbackInfo string) {
}

func (testFriendListener) OnFriendDeleted(callbackInfo string) {

}

func (testFriendListener) OnBlackAdded(callbackInfo string) {

}
func (testFriendListener) OnBlackDeleted(callbackInfo string) {

}

func (testFriendListener) OnFriendInfoChanged(callbackInfo string) {

}

func (testFriendListener) OnSuccess() {

}

func (testFriendListener) OnError(code int32, msg string) {

}

type testGroupListener struct {
}

func (testGroupListener) OnJoinedGroupAdded(callbackInfo string) {

}
func (testGroupListener) OnJoinedGroupDeleted(callbackInfo string) {

}

func (testGroupListener) OnGroupMemberAdded(callbackInfo string) {

}
func (testGroupListener) OnGroupMemberDeleted(callbackInfo string) {

}

func (testGroupListener) OnGroupApplicationAdded(callbackInfo string) {

}
func (testGroupListener) OnGroupApplicationDeleted(callbackInfo string) {

}

func (testGroupListener) OnGroupInfoChanged(callbackInfo string) {

}
func (testGroupListener) OnGroupMemberInfoChanged(callbackInfo string) {

}

func (testGroupListener) OnGroupApplicationAccepted(callbackInfo string) {

}
func (testGroupListener) OnGroupApplicationRejected(callbackInfo string) {

}

func (testGroupListener) OnGroupDismissed(callbackInfo string) {

}

type testConnListener struct {
}

func (t *testConnListener) OnUserTokenExpired() {

}
func (t *testConnListener) OnConnecting() {

}

func (t *testConnListener) OnConnectSuccess() {

}

func (t *testConnListener) OnConnectFailed(ErrCode int32, ErrMsg string) {

}

func (t *testConnListener) OnKickedOffline() {

}

func (t *testConnListener) OnSelfInfoUpdated(info string) {

}

func (t *testConnListener) OnSuccess() {

}

func (t *testConnListener) OnError(code int32, msg string) {

}
