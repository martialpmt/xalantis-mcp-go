#!/usr/bin/env python3
"""Test du serveur MCP : mock de l'API Xalantis + dialogue JSON-RPC via stdio."""
import json, os, re, subprocess, sys, tempfile, threading
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
        if u.path == "/api/v1/projects/p-1/documents/a-1/download":
            self.send_response(200)
            self.send_header("Content-Type", "application/octet-stream")
            self.send_header("Content-Disposition", 'attachment; filename="rapport.pdf"')
            self.end_headers()
            self.wfile.write(b"%PDF-test")
            return
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

    def do_POST(self):
        u = urlparse(self.path)
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        REQUESTS.append((self.path, self.headers.get("Authorization")))
        POSTS.append({"path": u.path, "idem": self.headers.get("Idempotency-Key"),
                      "type": self.headers.get("Content-Type"), "body": json.loads(body or b"null")})
        payload = json.dumps({"success": True, "data": {"uuid": "tk-1"}}).encode()
        self.send_response(201)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(payload)

POSTS = []
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
    {"jsonrpc": "2.0", "id": 11, "method": "tools/call",
     "params": {"name": "xalantis_search_operations", "arguments": {"query": "creer tache", "method": "POST"}}},
    {"jsonrpc": "2.0", "id": 12, "method": "tools/call",
     "params": {"name": "xalantis_describe_operation", "arguments": {"operation_id": "post_projects_By_projectUuid_tasks"}}},
    {"jsonrpc": "2.0", "id": 13, "method": "tools/call",
     "params": {"name": "xalantis_call_operation", "arguments": {
         "operation_id": "post_projects_By_projectUuid_tasks", "path_params": {"projectUuid": "p-1"},
         "headers": {"Idempotency-Key": "idem-42"}, "body": {"title": "Nouvelle tâche"}}}},
    {"jsonrpc": "2.0", "id": 14, "method": "tools/call",
     "params": {"name": "xalantis_read_operation", "arguments": {
         "operation_id": "get_projects_By_projectUuid_documents_By_attachmentUuid_download",
         "path_params": {"projectUuid": "p-1", "attachmentUuid": "a-1"}}}},
    {"jsonrpc": "2.0", "id": 15, "method": "tools/call",
     "params": {"name": "xalantis_call_operation", "arguments": {
         "operation_id": "post_projects_By_projectUuid_tasks", "path_params": {"projectUuid": "p-1"},
         "body": {"title": "sans clé"}}}},  # Idempotency-Key absente -> générée
    {"jsonrpc": "2.0", "id": 16, "method": "tools/call",
     "params": {"name": "xalantis_read_operation", "arguments": {
         "operation_id": "post_projects_By_projectUuid_tasks", "path_params": {"projectUuid": "p-1"}}}},  # écriture via l'outil de lecture -> erreur sans appel API
]

home = tempfile.mkdtemp()

def run_server(messages, **env):
    return subprocess.run(
        ["./xalantis-mcp-go"],
        input="".join(json.dumps(m) + "\n" for m in messages), capture_output=True, text=True, timeout=30,
        env={"XALANTIS_API_KEY": "sk_live_test", "XALANTIS_BASE_URL": f"http://127.0.0.1:{port}",
             "PATH": "/usr/bin", "HOME": home, **env},
    )

def responses(p):
    return {d.get("id"): d for d in (json.loads(l) for l in p.stdout.splitlines() if l.strip())}

proc = run_server(msgs)
resp = responses(proc)

ok = True
def check(cond, label):
    global ok
    print(("PASS " if cond else "FAIL ") + label)
    ok = ok and cond

check(resp[1]["result"]["serverInfo"]["name"] == "xalantis-mcp-go", "initialize")
check(resp[2]["result"] == {}, "ping")
annotations = {t["name"]: t.get("annotations", {}) for t in resp[3]["result"]["tools"]}
check(len(annotations) == 11, f"tools/list ({len(annotations)} outils)")
check(annotations["xalantis_read_operation"] == {"readOnlyHint": True}
      and annotations["xalantis_list_tasks"] == {"readOnlyHint": True}
      and annotations["xalantis_call_operation"] == {"destructiveHint": True}, "annotations des outils")
check("xalantis_read_operation" in resp[1]["result"].get("instructions", ""), "instructions d'initialisation")
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
r11 = json.loads(resp[11]["result"]["content"][0]["text"])
check(any(o["operation_id"] == "post_projects_By_projectUuid_tasks" for o in r11["operations"]), "search_operations")
r12 = json.loads(resp[12]["result"]["content"][0]["text"])
check(r12["method"] == "POST" and r12["body"]["content_type"] == "application/json"
      and any(p["name"] == "Idempotency-Key" and p["required"] for p in r12["parameters"]), "describe_operation")
r13 = json.loads(resp[13]["result"]["content"][0]["text"])
check(r13["data"]["uuid"] == "tk-1" and POSTS and POSTS[0] == {
    "path": "/api/v1/projects/p-1/tasks", "idem": "idem-42", "type": "application/json",
    "body": {"title": "Nouvelle tâche"}}, "call_operation POST JSON")
r14 = json.loads(resp[14]["result"]["content"][0]["text"])
saved = os.path.join(home, "Downloads", "xalantis", "rapport.pdf")
check(r14["saved_to"] == saved and open(saved, "rb").read() == b"%PDF-test", "read_operation téléchargement")
check(not resp[15]["result"]["isError"] and len(POSTS) == 2
      and re.fullmatch(r"[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}", POSTS[1]["idem"] or "")
      and POSTS[1]["body"] == {"title": "sans clé"}, "Idempotency-Key générée")
check(resp[16]["result"]["isError"] and "xalantis_call_operation" in resp[16]["result"]["content"][0]["text"],
      "read_operation refuse une écriture")
check(all(a == "Bearer sk_live_test" for _, a in REQUESTS), "header Authorization sur chaque appel")
check(len(REQUESTS) == 6, f"{len(REQUESTS)} appels API (les entrées invalides n'atteignent pas l'API)")
check(os.path.isdir(os.path.join(home, "Downloads", "xalantis")), "dossier XALANTIS_FILES_DIR créé au démarrage")
check(proc.stderr.strip() == "", "stderr vide")

ro_msgs = [msgs[0], {"jsonrpc": "2.0", "id": 2, "method": "tools/list"},
           {"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {
               "name": "xalantis_call_operation", "arguments": {"operation_id": "post_projects_By_projectUuid_tasks"}}}]
ro = responses(run_server(ro_msgs, XALANTIS_READ_ONLY="1"))
ro_tools = {t["name"] for t in ro[2]["result"]["tools"]}
check(len(ro_tools) == 10 and "xalantis_call_operation" not in ro_tools, "lecture seule : outil d'écriture absent")
check(ro[3]["result"]["isError"] and "outil inconnu" in ro[3]["result"]["content"][0]["text"],
      "lecture seule : appel d'écriture refusé")
check("XALANTIS_READ_ONLY" in ro[1]["result"]["instructions"], "lecture seule : instructions")
check(len(REQUESTS) == 6, "lecture seule : aucun appel API")
bad = run_server(msgs[:1], XALANTIS_READ_ONLY="oui")
check(bad.returncode == 1 and "XALANTIS_READ_ONLY" in bad.stderr, "XALANTIS_READ_ONLY invalide refusée")

srv.shutdown()
sys.exit(0 if ok else 1)
