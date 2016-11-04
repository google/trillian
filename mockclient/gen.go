package mockclient

//go:generate mockgen -self_package github.com/google/trillian/mockclient -package mockclient -destination mock_log_client.go github.com/google/trillian TrillianLogClient,TrillianLogServer,TrillianMapClient,TrillianMapServer
