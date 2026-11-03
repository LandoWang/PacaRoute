import { type Config } from "../api/types";
export type Common = {
  config: Config;
  save: (c: Config) => Promise<void>;
  close: () => void;
};
