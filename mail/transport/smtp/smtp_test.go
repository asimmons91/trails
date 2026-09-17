package smtp_test

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/asimmons91/trails/mail"
	"github.com/asimmons91/trails/mail/transport/smtp"
	"github.com/stretchr/testify/require"
)

// startFakeSMTP runs just enough of the SMTP protocol for net/smtp.SendMail
// to complete a plain (no AUTH, no STARTTLS) delivery, and reports the DATA
// body it received.
func startFakeSMTP(t *testing.T) (addr string, dataCh <-chan string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	ch := make(chan string, 1)

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		r := bufio.NewReader(conn)
		fmt.Fprintf(conn, "220 fake.smtp ready\r\n")

		var data strings.Builder
		inData := false

		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")

			if inData {
				if line == "." {
					inData = false
					ch <- data.String()
					fmt.Fprintf(conn, "250 OK\r\n")
					continue
				}
				data.WriteString(line + "\r\n")
				continue
			}

			switch upper := strings.ToUpper(line); {
			case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
				fmt.Fprintf(conn, "250-fake.smtp\r\n250 OK\r\n")
			case strings.HasPrefix(upper, "MAIL FROM"), strings.HasPrefix(upper, "RCPT TO"):
				fmt.Fprintf(conn, "250 OK\r\n")
			case upper == "DATA":
				inData = true
				fmt.Fprintf(conn, "354 Send data\r\n")
			case upper == "QUIT":
				fmt.Fprintf(conn, "221 Bye\r\n")
				return
			default:
				fmt.Fprintf(conn, "500 unrecognized command\r\n")
			}
		}
	}()

	return ln.Addr().String(), ch
}

func TestSendDeliversOverSMTP(t *testing.T) {
	addr, dataCh := startFakeSMTP(t)
	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)

	transport := smtp.New(smtp.Config{Host: host, Port: port})

	err = transport.Send(context.Background(), &mail.Message{
		From:    "from@example.com",
		To:      []string{"to@example.com"},
		Subject: "Hi",
		HTML:    "<p>hi</p>",
	})
	require.NoError(t, err)

	select {
	case data := <-dataCh:
		require.Contains(t, data, "From: from@example.com")
		require.Contains(t, data, "To: to@example.com")
		require.Contains(t, data, "<p>hi</p>")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the server to receive DATA")
	}
}

func TestSendReturnsErrorForCancelledContext(t *testing.T) {
	transport := smtp.New(smtp.Config{Host: "127.0.0.1", Port: 0})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := transport.Send(ctx, &mail.Message{From: "a@example.com", To: []string{"b@example.com"}, Text: "hi"})
	require.ErrorIs(t, err, context.Canceled)
}
