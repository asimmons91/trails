package mail

type Message struct {
	From    string
	To      []string
	Cc      []string
	Bcc     []string
	ReplyTo string
	Subject string
	Headers map[string]string

	HTML string
	Text string
}
