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
	UNSUPPORTED
)

var (
	ErrNotExist        = NewErr(NOT_EXIST, "Not exist")
	ErrNodeNotExist    = NewErr(NODE_NOT_EXIST, "Node not exist")
	ErrDriverNotExist  = NewErr(DRIVER_NOT_EXIST, "Driver not exist")
	ErrHostNotFound    = NewErr(HOST_NOT_EXIST, "Host not exist")
	ErrCommandNotExist = NewErr(COMMAND_NOT_EXIST, "Command not exist")
	ErrStorageNotExist = NewErr(STORAGE_NOT_EXIST, "Storage not exist")
	ErrTaskNotExist    = NewErr(TASK_NOT_EXIST, "Task not exist")

	ErrInvalidParameter = NewErr(INVALID_PARAMETER, "Invalid parameter")
	ErrLocked           = NewErr(LOCKED, "Locked")
	ErrUnlocked         = NewErr(UNLOCKED, "Unlocked")
	ErrConnection       = NewErr(CONNECTION_ERROR, "Could not connect")
	ErrAlreadyExist     = NewErr(ALREADY_EXIST, "Already exit")
	ErrOutOfQuota       = NewErr(OUTOF_QUATA, "Out of quota")
	ErrTimeout          = NewErr(TIMEOUT, "Timeout")
	ErrLoggerType       = NewErr(LOGGER_TYPE_ERROR, "Invalid logger type")
	ErrUnsupported      = NewErr(UNSUPPORTED, "unsupported")

	ErrNotTerminal = NewErr(NOT_TERMINAL, "Not terminal")
	ErrInvalidType = NewErr(INVALID_TYPE, "Invalid type")
	// ssh
	ErrSetDeadline   = NewErr(SET_DEADLINE_ERROR, "failed to set deadline")
	ErrSendKeepalive = NewErr(SEND_KEEPALIVE_ERROR, "failed to send keep alive")
	// etcd
	ErrETCDNotInit = NewErr(ETCD_NOT_INIT, "Etcd is not initialized")
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
