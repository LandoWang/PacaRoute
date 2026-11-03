import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { api } from "../api/transport";
import { type Config, type Status } from "../api/types";
export type AppState = {
  config: Config;
  status: Status;
  save: (c: Config) => Promise<void>;
  reload: () => Promise<void>;
  show: (v: ReactNode) => void;
  close: () => void;
  notice: (v: string) => void;
  navigate: (v: string) => void;
};
export const StateContext = createContext<AppState>(null!);
export const useApp = () => useContext(StateContext);
export function useLoad<T>(path: string, interval = 0) {
  const [data, setData] = useState<T>(),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true),
    [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    let pending = false;
    const load = async () => {
      if (pending) return;
      pending = true;
      try {
        const v = await api<T>(path, "GET", undefined, controller.signal);
        setData(v);
        setError("");
      } catch (e) {
        if (!controller.signal.aborted) setError((e as Error).message);
      } finally {
        pending = false;
        if (!controller.signal.aborted) setLoading(false);
      }
    };
    setLoading(true);
    setData(undefined);
    void load();
    const timer = interval
      ? setInterval(() => {
          if (!document.hidden) void load();
        }, interval)
      : undefined;
    return () => {
      controller.abort();
      clearInterval(timer);
    };
  }, [path, interval, revision]);
  return { data, error, loading, refresh: () => setRevision((v) => v + 1) };
}
