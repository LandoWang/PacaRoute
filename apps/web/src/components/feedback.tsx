import { RefreshCw } from "lucide-react";
import { Editor, ErrorNote } from "./ui";
export function Loading({
  loading,
  error,
}: {
  loading: boolean;
  error?: string;
}) {
  return (
    <>
      {loading && (
        <p className="muted loading">
          <RefreshCw className="spin" size={16} />
          加载中…
        </p>
      )}
      {error && <ErrorNote>{error}</ErrorNote>}
    </>
  );
}
export function Confirm({
  title,
  description,
  action,
  onClose,
}: {
  title: string;
  description: string;
  action: () => Promise<void>;
  onClose: () => void;
}) {
  return (
    <Editor
      title={title}
      onClose={onClose}
      onSave={async () => {
        await action();
        onClose();
      }}
      saveLabel="确认"
    >
      <p>{description}</p>
    </Editor>
  );
}
