import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { api } from "../api/transport";
import { type Config, type Status } from "../api/types";
import { startPolling } from "./polling";
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
    setLoading(true);
    setData(undefined);
    return startPolling(async (signal) => {
      try {
        const v = await api<T>(path, "GET", undefined, signal);
        setData(v);
        setError("");
      } catch (e) {
        if (!signal.aborted) setError((e as Error).message);
      } finally {
        if (!signal.aborted) setLoading(false);
      }
    }, interval);
  }, [path, interval, revision]);
  return { data, error, loading, refresh: () => setRevision((v) => v + 1) };
}
