package mysql

import (
	"github.com/go-sql-driver/mysql"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mysqlToGRPC converts some types of MySQL errors to GRPC errors. This gives
// clients more signal when the operation can be retried.
func mysqlToGRPC(err error) error {
	mysqlErr, ok := err.(*mysql.MySQLError)
	if !ok {
		return err
	}
	if mysqlErr.Number == 1213 { // ER_LOCK_DEADLOCK
		return status.Errorf(codes.Aborted, "MySQL: %v", mysqlErr)
	}
	return err
}
