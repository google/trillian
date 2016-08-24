package trillian

// Have to put this here, since I can't add it to the generated PB go code.
//go:generate mockgen -self_package github.com/google/trillian -package trillian -destination mock_log_client.go github.com/google/trillian TrillianLogClient,TrillianLogServer,TrillianMapClient,TrillianMapServer
