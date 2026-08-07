# MCP Catalog

A tiny catalog app for browsing Model Context Protocol (MCP) servers and their tools.

## Features

- register MCP server URLs
- discover available tools via `tools/list`
- refresh tool metadata
- invoke tools with JSON parameters

## Quick start

```bash
cd mcp-catalog
python -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -r requirements.txt
python app.py
```

Then open `http://127.0.0.1:5000`.

## Usage

1. Enter an MCP server URL.
2. Click `Add server`.
3. Browse discovered tools.
4. Run a tool by entering JSON parameters.
