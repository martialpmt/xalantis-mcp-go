#!/usr/bin/env python3
"""Test du serveur MCP : mock de l'API Xalantis + dialogue JSON-RPC via stdio."""
import json, subprocess, threading, sys
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlparse, parse_qs

REQUESTS = []

class Mock(BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        u = urlparse(self.path)
        REQUESTS.append((self.path, self.headers.get("Authorization")))
        routes = {
            "/api/v1/projects": {"success": True, "data": [
                {"uuid": "p-1", "key": "IAP", "name": "Intégrations & tests"},
                {"uuid": "p-2", "key": "CAP", "name": "Core API"}]},
            "/api/v1/projects/p-1/tasks": {"success": True, "data": [
                {"uuid": "t-1", "reference": "IAP-102", "title": "Test du mapping des codes d'erreur", "status": {"name": "Bloqué"}}],
                "meta": {"total": 1}},
            "/api/v1/projects/p-1/tasks/t-1": {"success": True, "data": {"uuid": "t-1", "reference": "IAP-102"}},
            "/api/v1/projects/p-1/tasks/t-1/activities": {"success": True, "data": [
                {"type": "status_changed", "from": "En cours", "to": "Bloqué", "at": "2026-08-28"}]},
            "/api/v1/projects/p-1/sprints": {"success": True, "data": []},
            "/api/v1/projects/p-1/statuses": {"success": True, "data": [{"uuid": "s-1", "name": "Bloqué"}]},
            "/api/v1/projects/p-1/members": {"success": True, "data": [{"uuid": "m-1", "name": "Salif Ka"}]},
        }
        if u.path == "/api/v1/forbidden":
            self.send_response(403); self.end_headers()
            self.wfile.write(b'{"message":"This API key does not have the required permission."}')
            return
        body = routes.get(u.path)
        if body is None:
            self.send_response(404); self.end_headers(); self.wfile.write(b'{"message":"not found"}')
            return
        payload = json.dumps({**body, "_query": parse_qs(u.query)}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(payload)

srv = HTTPServer(("127.0.0.1", 0), Mock)
port = srv.server_address[1]
threading.Thread(target=srv.serve_forever, daemon=True).start()

msgs = [
    {"jsonrpc": "2.0", "id": 1, "method": "initialize",
     "params": {"protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "test", "version": "0"}}},
    {"jsonrpc": "2.0", "method": "notifications/initialized"},
    {"jsonrpc": "2.0", "id": 2, "method": "ping"},
    {"jsonrpc": "2.0", "id": 3, "method": "tools/list"},
    {"jsonrpc": "2.0", "id": 4, "method": "tools/call",
     "params": {"name": "xalantis_list_projects", "arguments": {"search": "IAP"}}},
    {"jsonrpc": "2.0", "id": 5, "method": "tools/call",
     "params": {"name": "xalantis_list_tasks", "arguments": {
         "project_uuid": "p-1", "statuses": ["s-1"], "due_state": "overdue", "per_page": 50, "page": 2}}},
    {"jsonrpc": "2.0", "id": 6, "method": "tools/call",
     "params": {"name": "xalantis_task_activities", "arguments": {"project_uuid": "p-1", "task_uuid": "t-1"}}},
    {"jsonrpc": "2.0", "id": 7, "method": "tools/call",
     "params": {"name": "xalantis_list_tasks", "arguments": {}}},  # project_uuid manquant -> erreur outil
    {"jsonrpc": "2.0", "id": 8, "method": "tools/call",
     "params": {"name": "xalantis_get_task", "arguments": {"project_uuid": "p-1", "task_uuid": "../secret"}}},  # injection path
    {"jsonrpc": "2.0", "id": 9, "method": "unknown/method"},
    {"jsonrpc": "2.0", "id": 10, "method": "tools/call",
     "params": {"name": "xalantis_get_task", "arguments": {"project_uuid": "p-1", "task_uuid": ".."}}},  # segment ".."
    {"jsonrpc": "2.0", "method": "tools/call",
     "params": {"name": "xalantis_list_projects", "arguments": {}}},  # notification : ni réponse ni appel API
]
stdin_data = "".join(json.dumps(m) + "\n" for m in msgs)

proc = subprocess.run(
    ["./xalantis-projects-mcp"],
    input=stdin_data, capture_output=True, text=True, timeout=30,
    env={"XALANTIS_API_KEY": "sk_live_test", "XALANTIS_BASE_URL": f"http://127.0.0.1:{port}", "PATH": "/usr/bin"},
)
resp = {}
for line in proc.stdout.splitlines():
    if line.strip():
        d = json.loads(line)
        resp[d.get("id")] = d

ok = True
def check(cond, label):
    global ok
    print(("PASS " if cond else "FAIL ") + label)
    ok = ok and cond

check(resp[1]["result"]["serverInfo"]["name"] == "xalantis-projects-mcp", "initialize")
check(resp[2]["result"] == {}, "ping")
tools = {t["name"] for t in resp[3]["result"]["tools"]}
check(len(tools) == 7 and "xalantis_list_tasks" in tools, f"tools/list ({len(tools)} outils)")
r4 = json.loads(resp[4]["result"]["content"][0]["text"])
check(r4["_query"] == {"search": ["IAP"]} and r4["data"][0]["key"] == "IAP", "list_projects + query")
r5 = json.loads(resp[5]["result"]["content"][0]["text"])
q5 = r5["_query"]
check(q5.get("status[]") == ["s-1"] and q5.get("due_state") == ["overdue"]
      and q5.get("per_page") == ["50"] and q5.get("page") == ["2"], "list_tasks filtres status[]/due_state/pagination")
r6 = json.loads(resp[6]["result"]["content"][0]["text"])
check(r6["data"][0]["type"] == "status_changed", "task_activities")
check(resp[7]["result"]["isError"] and "project_uuid" in resp[7]["result"]["content"][0]["text"], "argument requis manquant")
check(resp[8]["result"]["isError"] and "invalide" in resp[8]["result"]["content"][0]["text"], "rejet injection de chemin")
check(resp[9]["error"]["code"] == -32601, "méthode inconnue")
check(resp[10]["result"]["isError"] and "invalide" in resp[10]["result"]["content"][0]["text"], "rejet segment ..")
check(None not in resp, "pas de réponse à une notification tools/call")
check(all(a == "Bearer sk_live_test" for _, a in REQUESTS), "header Authorization sur chaque appel")
check(len(REQUESTS) == 3, f"{len(REQUESTS)} appels API (les entrées invalides n'atteignent pas l'API)")
check(proc.stderr.strip() == "", "stderr vide")

srv.shutdown()
sys.exit(0 if ok else 1)
