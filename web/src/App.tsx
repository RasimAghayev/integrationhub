import { useCallback, useEffect, useState } from "react";
import {
  api,
  getToken,
  setToken,
  type Connection,
  type Execution,
  type Integration,
  type Schedule,
} from "./api";

type Tab = "connections" | "integrations" | "executions" | "schedules";

const TABS: Tab[] = ["connections", "integrations", "executions", "schedules"];

export function App() {
  const [tab, setTab] = useState<Tab>("integrations");
  const [token, setTokenState] = useState(getToken());
  const [apiKey, setApiKey] = useState("");
  const [error, setError] = useState("");
  const [connections, setConnections] = useState<Connection[]>([]);
  const [integrations, setIntegrations] = useState<Integration[]>([]);
  const [executions, setExecutions] = useState<Execution[]>([]);
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [scheduleForm, setScheduleForm] = useState({
    name: "",
    integration_id: "",
    endpoint: "",
    cron: "0 3 * * *",
  });

  const load = useCallback(async () => {
    setError("");
    try {
      const [conns, ints, execs, scheds] = await Promise.all([
        api.connections(),
        api.integrations(),
        api.executions(),
        api.schedules(),
      ]);
      setConnections(conns);
      setIntegrations(ints);
      setExecutions(execs);
      setSchedules(scheds);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, []);

  useEffect(() => {
    if (token) void load();
  }, [token, load]);

  async function signIn() {
    try {
      const res = await api.token(apiKey);
      setToken(res.token);
      setTokenState(res.token);
      setApiKey("");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function run(integration: Integration) {
    const endpoint = integration.endpoints[0]?.name;
    if (!endpoint) {
      setError(`${integration.name} has no endpoints`);
      return;
    }
    try {
      await api.execute(integration.id, endpoint);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function addSchedule() {
    try {
      await api.createSchedule({ ...scheduleForm, enabled: true });
      setScheduleForm({ name: "", integration_id: "", endpoint: "", cron: "0 3 * * *" });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }

  async function removeSchedule(id: string) {
    await api.deleteSchedule(id);
    await load();
  }

  async function replay(id: string) {
    await api.replay(id);
    await load();
  }

  if (!token) {
    return (
      <main className="shell">
        <h1>IntegrationHub</h1>
        <p className="hint">Sign in with an admin API key to obtain a JWT.</p>
        <div className="row">
          <input
            aria-label="Admin API key"
            placeholder="Admin API key"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
          />
          <button onClick={signIn}>Sign in</button>
        </div>
        {error && <p className="error">{error}</p>}
      </main>
    );
  }

  return (
    <main className="shell">
      <header className="topbar">
        <h1>IntegrationHub</h1>
        <button
          className="ghost"
          onClick={() => {
            setToken("");
            setTokenState("");
          }}
        >
          Sign out
        </button>
      </header>

      <nav className="tabs">
        {TABS.map((t) => (
          <button key={t} className={t === tab ? "active" : ""} onClick={() => setTab(t)}>
            {t}
          </button>
        ))}
        <button className="ghost" onClick={() => void load()}>
          refresh
        </button>
      </nav>

      {error && <p className="error">{error}</p>}

      {tab === "connections" && (
        <ul className="cards">
          {connections.map((c) => (
            <li key={c.id}>
              <strong>{c.name}</strong>
              <span>
                {c.type} · {c.auth_type ?? "none"}
              </span>
            </li>
          ))}
          {connections.length === 0 && <li className="empty">No connections yet.</li>}
        </ul>
      )}

      {tab === "integrations" && (
        <ul className="cards">
          {integrations.map((i) => (
            <li key={i.id}>
              <strong>{i.name}</strong>
              <span>
                {i.type} · {i.status} · {i.endpoints.length} endpoint(s)
              </span>
              <button onClick={() => void run(i)}>Run {i.endpoints[0]?.name ?? ""}</button>
            </li>
          ))}
          {integrations.length === 0 && <li className="empty">No integrations yet.</li>}
        </ul>
      )}

      {tab === "executions" && (
        <table>
          <thead>
            <tr>
              <th>id</th>
              <th>integration</th>
              <th>endpoint</th>
              <th>status</th>
              <th>code</th>
              <th>attempts</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {executions.map((e) => (
              <tr key={e.id}>
                <td className="mono">{e.id.slice(0, 8)}</td>
                <td>{e.integration_id.slice(0, 8)}</td>
                <td>{e.endpoint}</td>
                <td>{e.status}</td>
                <td>{e.status_code}</td>
                <td>{e.attempts}</td>
                <td>
                  {e.status === "dead" && <button onClick={() => void replay(e.id)}>replay</button>}
                </td>
              </tr>
            ))}
            {executions.length === 0 && (
              <tr>
                <td colSpan={7} className="empty">
                  No executions yet.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}

      {tab === "schedules" && (
        <>
          <div className="row">
            <input
              aria-label="Schedule name"
              placeholder="name"
              value={scheduleForm.name}
              onChange={(e) => setScheduleForm({ ...scheduleForm, name: e.target.value })}
            />
            <input
              aria-label="Integration id"
              placeholder="integration id"
              value={scheduleForm.integration_id}
              onChange={(e) =>
                setScheduleForm({ ...scheduleForm, integration_id: e.target.value })
              }
            />
            <input
              aria-label="Endpoint"
              placeholder="endpoint"
              value={scheduleForm.endpoint}
              onChange={(e) => setScheduleForm({ ...scheduleForm, endpoint: e.target.value })}
            />
            <input
              aria-label="Cron"
              placeholder="cron"
              value={scheduleForm.cron}
              onChange={(e) => setScheduleForm({ ...scheduleForm, cron: e.target.value })}
            />
            <button onClick={() => void addSchedule()}>Add</button>
          </div>
          <ul className="cards">
            {schedules.map((s) => (
              <li key={s.id}>
                <strong>{s.name}</strong>
                <span>
                  {s.cron} · {s.enabled ? "enabled" : "disabled"} · next {s.next_run}
                </span>
                <button onClick={() => void removeSchedule(s.id)}>delete</button>
              </li>
            ))}
            {schedules.length === 0 && <li className="empty">No schedules yet.</li>}
          </ul>
        </>
      )}
    </main>
  );
}