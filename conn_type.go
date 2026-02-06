package sshw

// ConnType represents the connection type
type ConnType int

const (
	ConnTypeSSH ConnType = iota
	ConnTypeSFTP
)

func (c ConnType) String() string {
	switch c {
	case ConnTypeSSH:
		return "SSH"
	case ConnTypeSFTP:
		return "SFTP"
	default:
		return "Unknown"
	}
}

func (c ConnType) Description() string {
	switch c {
	case ConnTypeSSH:
		return "Interactive SSH Shell"
	case ConnTypeSFTP:
		return "Interactive SFTP File Transfer"
	default:
		return ""
	}
}
