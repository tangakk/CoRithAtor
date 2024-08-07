package orchestrator

import (
	"context"
	"corithator/internal/sql"
	postfixnotation "corithator/pkg/postfix_notation"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	pb "corithator/proto"

	"github.com/dustinxie/lockfree"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
)

type OrchestratorConfig struct {
	Port             int    //если невалидный - меняется в 8080 при старте
	GRPC_Port        int    //порт грпц, если невалидный - меняется в 8081
	TimeLimitSeconds int    //по умолчанию - 60 секунд
	SecretKey        string //для JWT
	LoginTime        int    //время протухания JWT в минутах
}

type Orchestrator struct {
	Config          OrchestratorConfig //конфиг
	expressions     lockfree.Queue     //хранилище выражений в очереди
	results         lockfree.HashMap   //посчитанные и считаемые выражения
	agent_responses lockfree.Queue     //ответы агента
	tasks_to_give   lockfree.Queue     //задачи, чтобы дать агенту
	counter         int32              //считать айди
	pb.InternalServiceServer
}

type expression struct {
	id         int32   //айди
	expression []any   //выражение
	status     string  //статус
	result     float64 //результат
	user       string  //пользователь с выражением
}

func NewOrchestrator(config OrchestratorConfig) *Orchestrator {
	if config.Port <= 0 {
		config.Port = 8080
	}
	if config.GRPC_Port <= 0 {
		config.GRPC_Port = 8081
	}
	if config.TimeLimitSeconds <= 0 {
		config.TimeLimitSeconds = 60
	}
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
	sql.Init()
	defer sql.Close()
	r := chi.NewRouter()
	port_str := fmt.Sprint(o.Config.Port)
	r.Use(middleware.Logger)
	r.Use(middleware.URLFormat)

	go o.expressionProcessing()

	//собираем с бд всё неподсчитанное
	all_expr, err := sql.GetNotCountedExpressions()
	if err != nil {
		panic(err)
	}
	for _, e := range all_expr {
		tmp := expression{id: int32(e.Id), expression: arrayFromSqlToPfn(e.Expression)}
		o.expressions.Enque(tmp)
	}

	//все методы для API
	r.Route("/api", func(r chi.Router) {
		r.Use(o.loginMiddleware)
		r.Post("/calculate", o.addExpression)
		r.Get("/expressions", o.getExpressions)
		r.Get("/expressions/:{id}", o.getExpression)
	})

	//методы для регистрации/входа
	r.Route("/user", func(r chi.Router) {
		r.Post("/register", o.registerUser)
		r.Get("/login", o.loginUser)
	})

	//методы для посмтреть-потыкать
	r.Route("/foo", func(r chi.Router) {
		r.Get("/tasks", o.watchTasks)
	})

	go http.ListenAndServe(":"+port_str, r)

	//внутренние методы через gRPC

	host := "localhost"

	addr := fmt.Sprintf("%v:%v", host, o.Config.GRPC_Port)
	lis, err := net.Listen("tcp", addr)

	if err != nil {
		log.Println("error starting tcp listener: ", err)
		os.Exit(1)
	}

	log.Println("tcp listener started at port: ", o.Config.GRPC_Port)
	grpcServer := grpc.NewServer()

	pb.RegisterInternalServiceServer(grpcServer, o)

	if err := grpcServer.Serve(lis); err != nil {
		log.Println("error serving grpc: ", err)
		os.Exit(1)
	}
}

// Определяет, как распараллелить выражение
// А потом параллелит
// i don't like it
func (o *Orchestrator) expressionProcessing() {
	cnt := 0 //для айди задач
	for {
		t := o.expressions.Deque()
		if t == nil {
			continue
		}

		//это выражение считаем
		expr := t.(expression)
		changed := expr
		changed.status = "Processing"

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

		var err error
		timer := time.NewTimer(time.Second * time.Duration(o.Config.TimeLimitSeconds))
	BIG_LOOP:
		for len(expr_ordered) != 1 {
			select {
			case <-timer.C:
				err = errors.New("time limit exceeded")
				break BIG_LOOP
			default:

				//переберём всё
				for i, v := range expr_ordered {
					if i == 0 || i == len(expr_ordered)-1 {
						continue
					}

					arg1, ok1 := expr_ordered[i-1].data.(float64)
					arg2, ok2 := v.data.(float64)
					oper, ok3 := expr_ordered[i+1].data.(string)

					if ok1 && ok2 && ok3 {
						if oper == "/" && arg2 == 0 {
							err = errors.New("divison by zero")
							break BIG_LOOP
						}

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
							o.tasks_to_give.Enque(Task{
								Id:        cnt,
								Arg1:      arg1,
								Arg2:      arg2,
								Operation: oper,
							})
							cnt++
						}
					}
				}
				//перебрали, ура. Проверим, есть ли чё посчитанное агентом
				t = o.agent_responses.Deque()
				for t != nil {
					task_res := t.(TaskResult)
					pos, ok := tasks[task_res.Id]
					if !ok {
						continue
					}
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
		}
		//запишем ответ
		if err == nil {
			changed.result = expr_ordered[0].data.(float64)
			changed.status = "Ready"
		} else {
			changed.status = err.Error()
		}
		sql.WriteResult(int(changed.id), changed.result, changed.status)
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
	id, err := sql.AddExpression(fmt.Sprint(pfn), r.Context().Value("login").(string))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmp := expression{
		id:         int32(id),
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
	res := ExpressionGetResponse{Expressions: make([]ExpressionResp, 0)}
	expressions, err := sql.GetExpressions(r.Context().Value("login").(string))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, v := range expressions {
		res.Expressions = append(res.Expressions, ExpressionResp{
			Id:     int32(v.Id),
			Status: v.Status,
			Result: v.Result,
		})
	}
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (o *Orchestrator) getExpression(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	data_expr, err := sql.GetExpressionById(r.Context().Value("login").(string), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		fmt.Println(err)
		return
	}

	res := map[string]ExpressionResp{"expression": {
		int32(data_expr.Id),
		data_expr.Status,
		data_expr.Result}}
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

// выдача задач для агента
func (o *Orchestrator) TaskGet(ctx context.Context,
	in *pb.TaskGetRequest,
) (*pb.TaskGetResponse, error) {
	if o.tasks_to_give.Len() == 0 {
		return nil, errors.New("no tasks available")
	}
	task := o.tasks_to_give.Deque().(Task)
	res := &pb.TaskGetResponse{Id: int32(task.Id),
		Arg1:      float32(task.Arg1),
		Arg2:      float32(task.Arg2),
		Operation: task.Operation}
	fmt.Println(res)
	return res, nil
}

// получение ответа от агента
func (o *Orchestrator) TaskPost(ctx context.Context,
	in *pb.TaskPostRequest,
) (*pb.TaskPostResponse, error) {
	var taskRes = TaskResult{Id: int(in.Id), Result: float64(in.Result)}
	o.agent_responses.Enque(taskRes)
	return &pb.TaskPostResponse{}, nil
}

// регистрация пользователя
func (o *Orchestrator) registerUser(w http.ResponseWriter, r *http.Request) {
	var user UserRequest
	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	err = sql.RegisterUser(user.Login, user.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	res := map[string]string{"res": "ok"}
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (o *Orchestrator) loginUser(w http.ResponseWriter, r *http.Request) {
	var user UserRequest
	err := json.NewDecoder(r.Body).Decode(&user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	ok, err := sql.PasswordIsCorrect(user.Login, user.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if !ok {
		http.Error(w, "wrong password", http.StatusForbidden)
		return
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"login": user.Login,
		"nbf":   now.Unix(),
		"exp":   now.Add(time.Duration(o.Config.LoginTime) * time.Minute).Unix(),
		"iat":   now.Unix(),
	})
	tokenString, err := token.SignedString([]byte(o.Config.SecretKey))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	res := map[string]string{"JWT": tokenString}
	w.Header().Add("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (o *Orchestrator) loginMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenString := r.Header.Get("JWT")
		if tokenString == "" {
			http.Error(w, "bad JWT", http.StatusBadRequest)
			return
		}

		tokenFromString, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return []byte(o.Config.SecretKey), nil
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		claims := tokenFromString.Claims.(jwt.MapClaims)
		login := claims["login"].(string)

		if ok, _ := sql.IsUserExists(login); !ok {
			http.Error(w, "user does not exist", http.StatusBadRequest)
			return
		}

		newr := r.WithContext(context.WithValue(r.Context(), "login", login))
		next.ServeHTTP(w, newr)

	})
}

type CalculateRequest struct {
	Expression string `json:"expression"`
}

type TaskResult struct {
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

type UserRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
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

// конвертируем строчку в массив
func arrayFromSqlToPfn(str string) []any {
	str = str[1 : len(str)-1]
	str_arr := strings.Split(str, " ")
	arr := make([]any, len(str_arr))
	for i, v := range str_arr {
		if n, err := strconv.ParseFloat(v, 64); err != nil {
			arr[i] = v
		} else {
			arr[i] = n
		}
	}
	return arr
}
