package trip

import (
	"github.com/Masterminds/squirrel"

	"github.com/Znakous/template/internal/infra/postgres/transaction"
)

var psql = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

const tripColumns = "id, user_id, driver_id, start_latitude, start_longitude, end_latitude, end_longitude, price, status, started_at, finished_at"

type Trips struct {
	database transaction.Executor
}

func New(database transaction.Executor) *Trips {
	return &Trips{database: database}
}