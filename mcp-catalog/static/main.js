const { createApp } = Vue;

createApp({
  data() {
    return {
      servers: [],
      newServer: { url: "", name: "" },
      selectedServer: null,
      activeTool: null,
      toolParams: "{}",
      invokeResult: "",
      message: "",
    };
  },
  mounted() {
    this.loadServers();
  },
  methods: {
    async loadServers() {
      this.message = "Loading servers...";
      const res = await fetch("/api/servers");
      this.servers = await res.json();
      this.message = "";
      if (!this.selectedServer && this.servers.length) {
        this.selectedServer = this.servers[0];
      }
    },
    async addServer() {
      if (!this.newServer.url) {
        this.message = "Server URL is required.";
        return;
      }
      this.message = "Adding server...";
      const response = await fetch("/api/servers", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(this.newServer),
      });
      const data = await response.json();
      if (!response.ok) {
        this.message = data.error || "Unable to add server.";
        return;
      }
      this.servers.push(data);
      this.selectedServer = data;
      this.newServer = { url: "", name: "" };
      this.activeTool = null;
      this.invokeResult = "";
      this.message = "Server added.";
    },
    async deleteServer(server) {
      if (!confirm(`Delete ${server.name}?`)) return;
      await fetch(`/api/servers/${server.id}`, { method: "DELETE" });
      this.servers = this.servers.filter((item) => item.id !== server.id);
      if (this.selectedServer?.id === server.id) {
        this.selectedServer = this.servers[0] || null;
        this.activeTool = null;
        this.invokeResult = "";
      }
    },
    selectServer(server) {
      this.selectedServer = server;
      this.activeTool = null;
      this.invokeResult = "";
    },
    async refreshServer(server) {
      this.message = `Refreshing ${server.name}...`;
      const res = await fetch(`/api/servers/${server.id}/refresh`, { method: "POST" });
      const data = await res.json();
      if (!res.ok) {
        this.message = data.error || "Refresh failed.";
        return;
      }
      const index = this.servers.findIndex((item) => item.id === server.id);
      this.servers.splice(index, 1, data);
      if (this.selectedServer?.id === server.id) {
        this.selectedServer = data;
      }
      this.message = "Server refreshed.";
    },
    selectTool(tool) {
      this.activeTool = tool;
      this.toolParams = "{}";
      this.invokeResult = "";
    },
    async invokeTool() {
      let params;
      try {
        params = JSON.parse(this.toolParams || "{}");
      } catch (err) {
        this.invokeResult = "Invalid JSON parameters.";
        return;
      }
      const res = await fetch(`/api/servers/${this.selectedServer.id}/invoke`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ tool: this.activeTool, params }),
      });
      const data = await res.json();
      if (!res.ok) {
        this.invokeResult = data.error || "Invocation failed.";
        return;
      }
      this.invokeResult = JSON.stringify(data.result, null, 2);
    },
  },
}).mount("#app");
