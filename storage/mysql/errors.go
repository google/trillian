package mysql

import (
	"github.com/go-sql-driver/mysql"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// ER_DUP_ENTRY: Error returned by driver when inserting a duplicate row.
	errNumDuplicate = 1062
	// ER_LOCK_DEADLOCK: Error returned when there was a deadlock.
	errNumDeadlock = 1213
)

// mysqlToGRPC converts some types of MySQL errors to GRPC errors. This gives
// clients more signal when the operation can be retried.
func mysqlToGRPC(err error) error {
	mysqlErr, ok := err.(*mysql.MySQLError)
	if !ok {
		return err
	}
	if mysqlErr.Number == errNumDeadlock {
		return status.Errorf(codes.Aborted, "MySQL: %v", mysqlErr)
	}
	return err
}

func isDuplicateErr(err error) bool {
	switch err := err.(type) {
	case *mysql.MySQLError:
		return err.Number == errNumDuplicate
	default:
		return false
	}
}
