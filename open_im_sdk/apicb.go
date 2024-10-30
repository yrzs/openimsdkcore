package open_im_sdk

import (
	"context"
	"github.com/yrzs/openimsdkcore/open_im_sdk_callback"
	"github.com/yrzs/openimsdkcore/pkg/common"
	"github.com/yrzs/openimsdktools/errs"
	"github.com/yrzs/openimsdktools/log"
	"sync/atomic"
)

type apiErrCallback struct {
	loginMgrCh         chan common.Cmd2Value
	listener           open_im_sdk_callback.OnConnListener
	tokenExpiredState  int32
	kickedOfflineState int32
}

func (c *apiErrCallback) OnError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	codeErr, ok := errs.Unwrap(err).(errs.CodeError)
	if !ok {
		log.ZError(ctx, "OnError callback not CodeError", err)
		return
	}
	log.ZError(ctx, "OnError callback CodeError", err, "code", codeErr.Code(), "msg", codeErr.Msg(), "detail", codeErr.Detail())
	switch codeErr.Code() {
	case
		errs.TokenExpiredError,
		errs.TokenInvalidError,
		errs.TokenMalformedError,
		errs.TokenNotValidYetError,
		errs.TokenUnknownError,
		errs.TokenNotExistError:
		if atomic.CompareAndSwapInt32(&c.tokenExpiredState, 0, 1) {
			log.ZError(ctx, "OnUserTokenExpired callback", err)
			c.listener.OnUserTokenExpired()
			_ = common.TriggerCmdLogOut(ctx, c.loginMgrCh)
		}
	case errs.TokenKickedError:
		if atomic.CompareAndSwapInt32(&c.kickedOfflineState, 0, 1) {
			log.ZError(ctx, "OnKickedOffline callback", err)
			c.listener.OnKickedOffline()
			_ = common.TriggerCmdLogOut(ctx, c.loginMgrCh)
		}
	}
}
