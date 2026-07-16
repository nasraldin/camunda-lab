/** Human-friendly labels for Compose service names (Services + Logs). */
const FRIENDLY: Record<string, string> = {
  connectors: "Connectors",
  console: "Console",
  elasticsearch: "Elasticsearch",
  elasticvue: "ElasticVue",
  identity: "Identity",
  keycloak: "Keycloak",
  mailpit: "Mailpit",
  optimize: "Optimize",
  orchestration: "Orchestration",
  postgres: "Postgres",
  "web-modeler-db": "Web Modeler database",
  "web-modeler-restapi": "Web Modeler API",
  "web-modeler-websockets": "Web Modeler websockets",
  operate: "Operate",
  tasklist: "Tasklist",
  zeebe: "Zeebe",
};

export function friendlyName(service: string): string {
  return FRIENDLY[service] || service.replace(/-/g, " ");
}
