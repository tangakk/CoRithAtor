# CoRithAtor
(COncurrency aRITHmetic calculATOR)
Распределенный вычислитель арифметических выражений

## Установка и запуск:
Установите [git](https://git-scm.com/downloads) и [go](https://go.dev/dl/) (>= 1.22.3). Введите в терминал:
```bash
git clone https://github.com/tangakk/CoRithAtor.git
cd CoRithAtor
sh start.sh
```
Или, для Windows:
```bash
git clone https://github.com/tangakk/CoRithAtor.git
cd CoRithAtor
.\start.bat
```
После этого можете использовать start.sh или start.bat для запуска.

### Раздельный запуск

Вы можете запустить агент и оркестратор отдельно. Перейдите в папку, в которой находится репозиторий.

Для запуска оркестратора:
```bash
go run cmd/orchestrator/main.go
```
Для запуска агента:
```bash
go run cmd/agent/main.go
```

### Web-UI
Web-UI исключительно для демонстрации. Для запуска потребуется Python (>= 3.8)

Установите Gradio:
```bash
pip install gradio
```
или
```bash 
python3 -m pip install gradio
```
Запустите web-ui:
```
python3 web-ui/web-ui.py
```
В браузере по-умолчанию должна автоматически открыться страница. Если этого не произошло, то в терминале будет адрес, на котором находится Web-UI.
(Вам всё ещё нужно запустить оркестратор и агент, чтобы web-ui не был просто бесполезной страничкой)

## API и примеры запросов
Все методы API (которые вам можно использовать), находятся по адресу {orhcestrator_uri}/api/
### calculate
Добавляет выражение в очередь на обработку. Пример запроса:
```
curl --location 'localhost:8080/api/calculate' \
--header 'Content-Type: application/json' \
--data '{
      "expression": "2+5*8"
}'
```
Коды ответа:
- 201 - выражение принято
- 422 - невалидные данные; ответ содержит текст об ошибке

<!-- -->


В качестве ответа возвращает id выражения:
```
{ "id": 0 }
```
Примеры запросов с ответами:
```
curl --location 'localhost:8080/api/calculate' \
--header 'Content-Type: application/json' \
--data '{
      "expression": "4*(-2) + 2.5/7"
}'
```
Ответ:
{"id":1}
```
curl --location 'localhost:8080/api/calculate' \
--header 'Content-Type: application/json' \
--data '{
      "expression": "4*-2 + 2.5/7"
}'
```
Ответ: 
invalid expression: two signs in a row
```
curl --location 'localhost:8080/api/calculate' \
--header 'Content-Type: application/json' \
--data '{
      "expression": "8.5 + 6.7.8"
}'
```
Ответ:
invalid expression: two points in one number
### expressions
Возвращает информацию о всех выражениях, которые считались или будут считаться.
```
curl --location 'localhost:8080/api/expressions'
```
Ответ:
```
{"expressions":
  [{"id":1,"status":"Ready","result":15.280000000000001},
  {"id":0,"status":"Ready","result":-7.642857142857143}]
}
```
### expressions/:{id}
Подставьте нужный id и получите информацию о конкретном выражении
```
curl --location 'localhost:8080/api/expressions/:1'
```
Коды ответа:
- 200 - выражение найдено
- 404 - выражение не найдено
- 422 - невалидный id

<!-- -->

Пример ответа:
```
{"expression":{"id":1,"status":"Ready","result":15.280000000000001}}
```
## Внутренние методы
Агент и оркестратор общаются друг с другом через gRPC. Можете просмотреть [proto файл](proto/internal.proto), тут описания не будет

<!-- -->

## Параметры
Параметры хранятся в папке settings
### Параметры агента
Параметры агента хранятся в agent.json
- "OrchestratorURI": путь до оркестратора
- "TaskPath": путь до метода task
- "MaxWorkers": количество операций, которые могут выполняться параллельно; может задаваться флагом -COMPUTING_POWER при запуске
- "DelayMs": задержка между GET-запросами агента к оркестратору
- "Time...Ms": определяют время на выполнение одноименных операций
### Параметры оркестратора
Параметры оркестратора хранятся в orchestrator.json
- "Port": порт, на котором работает оркестратор; может задаваться флагом -port при запуске
- "TimeLimitSeconds": время в секундах, по истечении которого прекращаются попытки посчитать выражение
## Принцип работы
1) При запросе calculate Оркестратор проверяет выражение на корректность, затем делит на подзадачи, которые можно выполнить параллельно
2) Агент постоянно посылает GET-запросы task. Когда у Оркестратора появляются подзадачи, он выдаёт их Агенту.
3) Агент вычисляет ответы на подзадачи и отправляет их Оркестратору POST-запросом task
4) Оркестратор заменяет в выражении подзадачу на её ответ, после чего снова пытается делить выражение на подзадачи.
5) Когда в выражении не остаётся ничего, кроме одного числа, Оркестратор записывает результат выражения.

<!-- -->

То, как работают отдельные элементы (например, деление на подзадачи), описано в комментариях в коде. Просто не факт, что вы захотите его читать.
