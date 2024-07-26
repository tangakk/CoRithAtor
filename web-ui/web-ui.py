import gradio as gr
import requests
import time

jwt = ""

def calculate(expression):
    data = {"expression": expression}
    headers = {"Jwt":jwt}
    r = requests.post("http://localhost:8080/api/calculate", json=data, headers=headers)
    print(r.request.headers)
    if r.status_code!=201:
        return r.text
    id = r.json()["id"]
    r = requests.get(f"http://localhost:8080/api/expressions/:{id}", headers=headers)
    if r.status_code!=200:
        return r.text
    r_json = r.json()["expression"]
    while r_json["status"] == "In queue" or r_json["status"] == "Processing":
        time.sleep(1)
        r = requests.get(f"http://localhost:8080/api/expressions/:{id}", headers=headers)
        if r.status_code!=200:
            return r.text
        r_json = r.json()["expression"]
    if r_json["status"]=="Ready":
        return r_json["result"]
    return r_json["status"]

def getResults():
    headers = {"Jwt":jwt}
    r = requests.get("http://localhost:8080/api/expressions", headers=headers)
    if r.status_code!=200:
        return r.text
    expressions = list(r.json()["expressions"])
    expressions.sort(key=lambda x: x["id"])
    return expressions

def getById(id):
    headers = {"Jwt":jwt}
    r = requests.get(f"http://localhost:8080/api/expressions/:{id}",headers = headers)
    if r.status_code!=200:
        return r.text
    return r.json()

def register(login, password):
    data = {"login": login, "password": password}
    r = requests.post("http://localhost:8080/user/register", json=data)
    if r.status_code!=200:
        return r.text
    return r.json()

def login(login, password):
    global jwt
    data = {"login": login, "password": password}
    r = requests.get("http://localhost:8080/user/login", json=data)
    if r.status_code!=200:
        return r.text
    jwt = r.json()["JWT"]
    print(jwt)
    return "Succesfull login"


with gr.Blocks() as demo:
    inp = gr.Textbox(placeholder="Напишите выражение", label="Input")
    out = gr.Textbox(label="Output")
    calc_butt = gr.Button("Calculate")
    calc_butt.click(calculate, inputs=inp, outputs=out)
    with gr.Row():
        with gr.Column():
            gr.Markdown("Получить все результаты")
            out_expressions = gr.Textbox(label="Output")
            get_butt = gr.Button("Get")
            get_butt.click(getResults, outputs=out_expressions)
        with gr.Column():
            gr.Markdown("Получить результат по айди")
            inp_id = gr.Textbox(label="Id")
            out_expressions_by_id = gr.Textbox(label="Output")
            get_id_butt = gr.Button("Get")
            get_id_butt.click(getById, inputs=inp_id, outputs=out_expressions_by_id)
    with gr.Row():
        with gr.Column():
            login_ = gr.Textbox(placeholder="login", label="Login")
            password = gr.Textbox(placeholder="password", label="Password")
            res = gr.Textbox(label="Results")
            with gr.Row():
                reg_butt = gr.Button("Register")
                login_butt = gr.Button("Login")
            reg_butt.click(register, inputs=[login_, password], outputs=res)
            login_butt.click(login, inputs=[login_, password], outputs=res)

demo.launch(inbrowser=True)