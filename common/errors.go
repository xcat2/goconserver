package common

const (
	// NOT exist
	NOT_EXIST = 1
	NODE_NOT_EXIST
	DRIVER_NOT_EXIST
	COMMAND_NOT_EXIST
	STORAGE_NOT_EXIST
	TASK_NOT_EXIST

	INVALID_PARAMETER
	LOCKED
	CONNECTION_ERROR
	ALREADY_EXIST
	OUTOF_QUATA
	TIMEOUT
	UNLOCKED
	NOT_TERMINAL
	HOST_NOT_EXIST
	INVALID_TYPE
	ETCD_NOT_INIT
	SET_DEADLINE_ERROR
	SEND_KEEPALIVE_ERROR
	LOGGER_TYPE_ERROR

	// error message
	STRING_NOT_EXIST         = "Not exist"
	STRING_NODE_NOT_EXIST    = "Node not exist"
	STRING_DRIVER_NOT_EXIST  = "Driver not exist"
	STRING_HOST_NOT_EXIST    = "Host not exist"
	STRING_COMMAND_NOT_EXIST = "Command not exist"
	STRING_STORAGE_NOT_EXIST = "Storage not exist"
	STRING_TASK_NOT_EXIST    = "TASK_NOT_EXIST"

	STRING_INVALID_PARAMETER = "Invalid parameter"
	STRING_LOCK              = "Locked"
	STRING_UNLOCK            = "Unlocked"
	STRING_CONNECTION_ERROR  = "Could not connect"
	STRING_ALREADY_EXIST     = "Already exit"
	STRING_OUTOF_QUOTA       = "Out of quota"
	STRING_TIMEOUT           = "Timeout"
	STRING_LOGGER_TYPE_ERROR = "Invalid logger type"

	STRING_NOT_TERMINAL  = "Not terminal"
	STRING_INVALID_TYPE  = "Invalid type"
	STRING_ETCD_NOT_INIT = "Etcd is not initialized"
	//ssh
	STRING_SET_DEADLINE_ERROR   = "failed to set deadline"
	STRING_SEND_KEEPALIVE_ERROR = "failed to send keep alive"
)

var (
	ErrNotExist        = NewErr(NOT_EXIST, STRING_NOT_EXIST)
	ErrNodeNotExist    = NewErr(NODE_NOT_EXIST, STRING_NODE_NOT_EXIST)
	ErrDriverNotExist  = NewErr(DRIVER_NOT_EXIST, STRING_DRIVER_NOT_EXIST)
	ErrHostNotFound    = NewErr(HOST_NOT_EXIST, STRING_HOST_NOT_EXIST)
	ErrCommandNotExist = NewErr(COMMAND_NOT_EXIST, STRING_COMMAND_NOT_EXIST)
	ErrStorageNotExist = NewErr(STORAGE_NOT_EXIST, STRING_STORAGE_NOT_EXIST)
	ErrTaskNotExist    = NewErr(TASK_NOT_EXIST, STRING_TASK_NOT_EXIST)

	ErrInvalidParameter = NewErr(INVALID_PARAMETER, STRING_INVALID_PARAMETER)
	ErrLocked           = NewErr(LOCKED, STRING_LOCK)
	ErrUnlocked         = NewErr(UNLOCKED, STRING_UNLOCK)
	ErrConnection       = NewErr(CONNECTION_ERROR, STRING_CONNECTION_ERROR)
	ErrAlreadyExist     = NewErr(ALREADY_EXIST, STRING_ALREADY_EXIST)
	ErrOutOfQuota       = NewErr(OUTOF_QUATA, STRING_OUTOF_QUOTA)
	ErrTimeout          = NewErr(TIMEOUT, STRING_TIMEOUT)
	ErrLoggerType       = NewErr(LOGGER_TYPE_ERROR, STRING_LOGGER_TYPE_ERROR)

	ErrNotTerminal = NewErr(NOT_TERMINAL, STRING_NOT_TERMINAL)
	ErrInvalidType = NewErr(INVALID_TYPE, STRING_INVALID_TYPE)
	// ssh
	ErrSetDeadline   = NewErr(SET_DEADLINE_ERROR, STRING_SET_DEADLINE_ERROR)
	ErrSendKeepalive = NewErr(SEND_KEEPALIVE_ERROR, STRING_SEND_KEEPALIVE_ERROR)
	// etcd
	ErrETCDNotInit = NewErr(ETCD_NOT_INIT, STRING_ETCD_NOT_INIT)
)

func NewErr(code int, text string) *GoConsError {
	return &GoConsError{code: code, s: text}
}

type GoConsError struct {
	code int
	s    string
}

func (e *GoConsError) Error() string {
	return e.s
}
