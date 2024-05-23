# CoRithAtor
(COncurrency aRITHmetic calculATOR)
Распределенный вычислитель арифметических выражений

## Установка и запуск:
Linux: установите git и go (>= 1.22.3). Введите в терминал:
```bash
git clone https://github.com/tangakk/CoRithAtor.git
cd CoRithAtor
sh start.sh
```

Windows: тоже установите git и go(>= 1.22.3). Введите в терминал:
```bash
git clone https://github.com/tangakk/CoRithAtor.git
cd CoRithAtor
start.bat
```

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
В браузере по-умолчанию должна автоматичесик открыться страница. Если этого не произошло, то в терминале будет адрес, на котором находится Web-UI.
