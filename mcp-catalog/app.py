import json
import os
import time
from pathlib import Path

import requests
from flask import Flask, jsonify, request, render_template, abort

BASE_DIR = Path(__file__).resolve().parent
CATALOG_PATH = BASE_DIR / "catalog.json"

app = Flask(__name__, template_folder="templates", static_folder="static")


def load_catalog():
    if CATALOG_PATH.exists():
        try:
            return json.loads(CATALOG_PATH.read_text())
        except json.JSONDecodeError:
            return {"servers": []}
    return {"servers": []}


def save_catalog(catalog):
    CATALOG_PATH.write_text(json.dumps(catalog, indent=2))


def build_payload(method, params=None):
    return {
        "jsonrpc": "2.0",
        "id": int(time.time() * 1000),
        "method": method,
        "params": params or {},
    }


def call_mcp(url, method, params=None):
    payload = build_payload(method, params)
    response = requests.post(url, json=payload, timeout=15)
    response.raise_for_status()
    data = response.json()
    if "error" in data:
        error = data["error"]
        raise ValueError(f"MCP error {error.get('code')}: {error.get('message')}")
    return data.get("result")


@app.route("/")
def index():
    return render_template("index.html")


@app.route("/api/servers", methods=["GET"])
def list_servers():
    catalog = load_catalog()
    return jsonify(catalog)


@app.route("/api/servers", methods=["POST"])
def add_server():
    body = request.get_json(silent=True) or {}
    url = body.get("url")
    name = body.get("name") or url
    if not url:
        return jsonify({"error": "Missing server URL"}), 400

    try:
        result = call_mcp(url, "tools/list")
    except Exception as exc:
        return jsonify({"error": str(exc)}), 400

    tools = [tool.get("name") for tool in result.get("tools", [])]
    catalog = load_catalog()
    next_id = max((server["id"] for server in catalog["servers"]), default=0) + 1
    entry = {
        "id": next_id,
        "name": name,
        "url": url,
        "tools": result.get("tools", []),
        "tool_names": tools,
        "last_discovered": int(time.time()),
    }
    catalog["servers"].append(entry)
    save_catalog(catalog)
    return jsonify(entry), 201


@app.route("/api/servers/<int:server_id>", methods=["DELETE"])
def delete_server(server_id):
    catalog = load_catalog()
    servers = [server for server in catalog["servers"] if server["id"] != server_id]
    if len(servers) == len(catalog["servers"]):
        return jsonify({"error": "Server not found"}), 404
    catalog["servers"] = servers
    save_catalog(catalog)
    return jsonify({"status": "deleted"})


@app.route("/api/servers/<int:server_id>/refresh", methods=["POST"])
def refresh_server(server_id):
    catalog = load_catalog()
    server = next((s for s in catalog["servers"] if s["id"] == server_id), None)
    if not server:
        return jsonify({"error": "Server not found"}), 404

    try:
        result = call_mcp(server["url"], "tools/list")
    except Exception as exc:
        return jsonify({"error": str(exc)}), 400

    server["tools"] = result.get("tools", [])
    server["tool_names"] = [tool.get("name") for tool in server["tools"]]
    server["last_discovered"] = int(time.time())
    save_catalog(catalog)
    return jsonify(server)


@app.route("/api/servers/<int:server_id>/invoke", methods=["POST"])
def invoke_tool(server_id):
    body = request.get_json(silent=True) or {}
    tool_name = body.get("tool")
    params = body.get("params", {})
    if not tool_name:
        return jsonify({"error": "tool is required"}), 400

    catalog = load_catalog()
    server = next((s for s in catalog["servers"] if s["id"] == server_id), None)
    if not server:
        return jsonify({"error": "Server not found"}), 404

    try:
        result = call_mcp(server["url"], tool_name, params)
    except Exception as exc:
        return jsonify({"error": str(exc)}), 400

    return jsonify({"result": result})


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=5000, debug=True)
