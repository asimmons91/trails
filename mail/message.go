package mail

// Message is the mail envelope passed to Deliver and Transport.Send.
type Message struct {
	// From is the sender address. Deliver fills this in from the Mailer's
	// default when left empty.
	From    string
	To      []string
	Cc      []string
	Bcc     []string
	ReplyTo string
	Subject string
	// Headers are extra headers merged into the message BuildRFC822
	// builds, alongside the standard ones it sets itself.
	Headers map[string]string

	// HTML and Text are the rendered bodies. Deliver overwrites both from
	// the template it renders; set them directly only when sending a
	// Message without going through Deliver.
	HTML string
	Text string
}
