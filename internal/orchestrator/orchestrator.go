package orchestrator

import (
	postfixnotation "corithator/pkg/postfix_notation"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/dustinxie/lockfree"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type OrchestratorConfig struct {
	Port int //если невалидный - меняется в 8080 при старте
}

type Orchestrator struct {
	Config          OrchestratorConfig //конфиг
	expressions     lockfree.Queue     //хранилище выражений в очереди
	results         lockfree.HashMap   //посчитанные и считаемые выражения
	agent_responses lockfree.Queue     //ответы агента
	tasks_to_give   lockfree.Queue     //задачи, чтобы дать агенту
	counter         int32              //считать айди
}

type expression struct {
	id         int32   //айди
	expression []any   //выражение
	status     string  //статус
	result     float64 //результат
}

func NewOrchestrator(config OrchestratorConfig) *Orchestrator {
	return &Orchestrator{
		Config:          config,
		expressions:     lockfree.NewQueue(),
		results:         lockfree.NewHashMap(),
		agent_responses: lockfree.NewQueue(),
		tasks_to_give:   lockfree.NewQueue(),
		counter:         0}
}

// запускает оркестратор
func (o *Orchestrator) Run() {
	r := chi.NewRouter()
	if o.Config.Port <= 0 {
		o.Config.Port = 8080
	}
	port_str := fmt.Sprint(o.Config.Port)
	r.Use(middleware.Logger)
	r.Use(middleware.URLFormat)

	go o.expressionProcessing()

	//все методы для API
	r.Route("/api", func(r chi.Router) {
		r.Post("/calculate", o.addExpression)
		r.Get("/expressions", o.getExpressions)
		r.Get("/expressions/:{id}", o.getExpression)
	})

	//внутренние методы
	r.Route("/internal", func(r chi.Router) {
		r.Get("/task", o.getTask)
		r.Post("/task", o.postTaskResult)
	})

	//методы для посмтреть-потыкать
	r.Route("/foo", func(r chi.Router) {
		r.Get("/tasks", o.watchTasks)
	})

	http.ListenAndServe(":"+port_str, r)
}

// Определяет, как распараллелить выражение
// А потом параллелит
// i don't like it
func (o *Orchestrator) expressionProcessing() {
	for {
		t := o.expressions.Deque()
		if t == nil {
			continue
		}

		//это выражение считаем
		expr := t.(expression)
		changed := expr
		changed.status = "Processing"
		o.results.Set(expr.id, changed)

		tasks := make(map[int]int) //ключ - айди задачи, значение - место в выражении

		//Принцип работы:
		//Каждое выражение - слайс чисел и операторов в постфиксной записи
		//Параллельно можно считать все такие места в записи, где подряд идут два числа и оператор
		//Результатом заменяем эти три элемента в записи, повторяем пока в записи >1 элемента

		//Пример: выражение 5*6+(2-9)
		//В постфиксной записи: 5 6 * 2 9 - +
		//Параллельно считаем 5 6 * и 2 9 -
		//Получаем ответы 30 и -7
		//Заменяем ответами изначальные выражения, получаем 30 -7 +
		//Считаем
		//Ответ: 23

		//чтобы не потеряться, буду хранить позицию в изначальном выражении
		type exprElement struct {
			pos  int //позиция в изнач. выражении
			data any //сам элемент
		}
		expr_ordered := make([]exprElement, len(expr.expression))
		for i, j := range expr.expression {
			expr_ordered[i] = exprElement{pos: i, data: j}
		}

		cnt := 0 //для айди задач
		for len(expr_ordered) != 1 {
			//переберём всё
			for i, v := range expr_ordered {
				if i == 0 || i == len(expr_ordered)-1 {
					continue
				}

				arg1, ok1 := expr_ordered[i-1].data.(float64)
				arg2, ok2 := v.data.(float64)
				oper, ok3 := expr_ordered[i+1].data.(string)
				if ok1 && ok2 && ok3 {
					//проверим, не считается ли это уже
					task_already_started := false
					for _, pos := range tasks {
						if pos == v.pos {
							task_already_started = true
							break
						}
					}
					//если не считается ещё
					if !task_already_started {
						tasks[cnt] = v.pos
						o.tasks_to_give.Enque(TaskGetResponse{Task: Task{
							Id:        cnt,
							Arg1:      arg1,
							Arg2:      arg2,
							Operation: oper,
						}})
						cnt++
					}
				}
			}
			//перебрали, ура. Проверим, есть ли чё посчитанное агентом
			t = o.agent_responses.Deque()
			for t != nil {
				task_res := t.(TaskPostRequest)
				pos := tasks[task_res.Id]
				delete(tasks, task_res.Id)
				//найдём, где эта позиция сейчас в выражении
				for i, v := range expr_ordered {
					if v.pos == pos {
						//и заменим на результат, остальное удалим
						expr_ordered[i].data = task_res.Result
						expr_ordered = remove(expr_ordered, i+1)
						expr_ordered = remove(expr_ordered, i-1)
					}
				}
				t = o.agent_responses.Deque()
			}
		}
		//запишем ответ
		changed.result = expr_ordered[0].data.(float64)
		changed.status = "Ready"
		o.results.Set(changed.id, changed)
	}
}

// добавляет выражение для обработки
func (o *Orchestrator) addExpression(w http.ResponseWriter, r *http.Request) {
	var calculateReq CalculateRequest
	err := json.NewDecoder(r.Body).Decode(&calculateReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	pfn, err := postfixnotation.ToPostfixNotation(calculateReq.Expression)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	tmp := expression{
		id:         o.counter,
		expression: pfn,
		status:     "In queue",
		result:     0,
	}

	o.expressions.Enque(tmp)
	o.results.Set(tmp.id, tmp)

	w.WriteHeader(http.StatusCreated)
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int32{"id": tmp.id})
	atomic.AddInt32(&o.counter, 1)
}

func (o *Orchestrator) getExpressions(w http.ResponseWriter, r *http.Request) {
	expressions := ExpressionGetResponse{Expressions: make([]ExpressionResp, o.results.Len())}
	o.results.Lock()
	cnt := 0
	for _, v, ok := o.results.Next(); ok; _, v, ok = o.results.Next() {
		//fmt.Println(o.results.Get(k))
		v_exp := v.(expression)
		expressions.Expressions[cnt] = ExpressionResp{v_exp.id, v_exp.status, v_exp.result}
		cnt++
	}
	o.results.Unlock()
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(expressions)
}

func (o *Orchestrator) getExpression(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	data, ok := o.results.Get(int32(id))
	if !ok {
		http.Error(w, "no expression with such id found: "+chi.URLParam(r, "id"), http.StatusNotFound)
		return
	}
	data_expr := data.(expression)

	res := map[string]ExpressionResp{"expression": {data_expr.id, data_expr.status, data_expr.result}}
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// выдача задач для агента
func (o *Orchestrator) getTask(w http.ResponseWriter, r *http.Request) {
	if o.tasks_to_give.Len() == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	task := o.tasks_to_give.Deque().(TaskGetResponse)
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

// получение ответа от агента
func (o *Orchestrator) postTaskResult(w http.ResponseWriter, r *http.Request) {
	var taskPostReq TaskPostRequest
	err := json.NewDecoder(r.Body).Decode(&taskPostReq)
	if err != nil {
		http.Error(w, "invalid data", http.StatusUnprocessableEntity)
	}
	o.agent_responses.Enque(taskPostReq)
	w.WriteHeader(200)
}

type CalculateRequest struct {
	Expression string `json:"expression"`
}

type TaskPostRequest struct {
	Id     int     `json:"id"`
	Result float64 `json:"result"`
}

type TaskGetResponse struct {
	Task Task `json:"task"`
}

type Task struct {
	Id        int     `json:"id"`
	Arg1      float64 `json:"arg1"`
	Arg2      float64 `json:"arg2"`
	Operation string  `json:"operation"`
}

type ExpressionGetResponse struct {
	Expressions []ExpressionResp `json:"expressions"`
}

type ExpressionResp struct {
	Id     int32   `json:"id"`
	Status string  `json:"status"`
	Result float64 `json:"result"`
}

// удалить из слайса
func remove[T any](slice []T, s int) []T {
	return append(slice[:s], slice[s+1:]...)
}

// посмотреть выданные задачи
// ломает к чертям подсчёт выражения, применять только для потыкать-посмотреть
func (o *Orchestrator) watchTasks(w http.ResponseWriter, r *http.Request) {
	for range o.tasks_to_give.Len() {
		t := o.tasks_to_give.Deque()
		task := t.(TaskGetResponse).Task
		w.Write([]byte(fmt.Sprint(task.Id, task.Arg1, task.Operation, task.Arg2, "\n")))
		o.tasks_to_give.Enque(t.(TaskGetResponse))
	}
}
