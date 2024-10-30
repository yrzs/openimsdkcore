package msgtest

import (
	"fmt"
	"github.com/yrzs/openimsdkcore/pkg/constant"
	"github.com/yrzs/openimsdkcore/sdk_struct"
)

// config here

// system
var (
	TESTIP        = "59.36.173.89"
	APIADDR       = fmt.Sprintf("http://%v:10002", TESTIP)
	WSADDR        = fmt.Sprintf("ws://%v:10001", TESTIP)
	SECRET        = "openIM123"
	MANAGERUSERID = "openIMAdmin"

	PLATFORMID = constant.WindowsPlatformID
	LogLevel   = uint32(5)

	REGISTERADDR = APIADDR + constant.UserRegister
	TOKENADDR    = APIADDR + constant.GetUsersToken
)

func GetConfig() *sdk_struct.IMConfig {
	var cf sdk_struct.IMConfig
	cf.ApiAddr = APIADDR
	cf.PlatformID = int32(PLATFORMID)
	cf.WsAddr = WSADDR
	cf.DataDir = "./"
	cf.LogLevel = LogLevel
	cf.IsExternalExtensions = true
	cf.IsLogStandardOutput = true
	cf.LogFilePath = ""
	return &cf

}
