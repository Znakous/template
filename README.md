## инструкции
```sh
tripgoctl cluster start
tripgoctl environment start
tripgoctl connect
```
накат
```sh
make migrate
```
запуск
```sh
make run
```

убить
```sh
make kill
```

тесты
```sh
go test ./...
```

## вопросы

### Как работает менеджер транзакций?
Менеджер стартует транзакцию, pgx.Tx отправляется в контекст, репы будут просить executor через ExecutorFromContext - когда мы в транзакции это транзакция, иначе пул

### Что будет при двух одновременных finish?
Там UPDATE ... WHERE id = $1 AND status = 'active' и поэтому всё круто

### Зачем /ready отдельно от /health и почему /health не ходит в базу
Health - алё а в этой поде что-то живое есть, что нам может ответить

Ready - ты точно не отдашь фигню, если данные будут валидные / ты готов обрабатывать запрос. Без пинга БД не можем гарантировать, что всё будет ок и на всё не прилетит 500

## Переменные
HTTP_ADDR=:8080 ибо нефиг

LOG_LEVEL=info debug отключаем, мы в проде

SHUTDOWN_TIMEOUT=10s логично

DATABASE_URL=postgres://tripgo:tripgo@localhost:21032/tripgo?sslmode=disable ну урла

DATABASE_MAX_CONNS=10 подбирается эмпирически

DATABASE_MIN_CONNS=2 туда же

DATABASE_MAX_CONN_LIFETIME=30m и это тоже

DATABASE_CONNECT_TIMEOUT=5s ...

DATABASE_QUERY_TIMEOUT=3s ...


## Что сделано в лабе 

Все что было в основном условии + Idempotency key