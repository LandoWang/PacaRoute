import { Copy, Check } from "lucide-react";
import { useState } from "react";
import { Button, Badge, Modal, ErrorNote } from "../../components/ui";
export function Secret({
  value,
  id,
  onClose,
}: {
  value: string;
  id: string;
  onClose: () => void;
}) {
  const [copied, setCopied] = useState(false),
    [error, setError] = useState("");
  return (
    <Modal
      title="保存客户端密钥"
      description="密钥已加密保存，接入脚本会自动包含它。请只交给对应调用方。"
      onClose={onClose}
    >
      <div className="form-body">
        <Badge tone="mint">{id}</Badge>
        <pre className="secret">{value}</pre>
        <Button
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(value);
              setCopied(true);
            } catch {
              setError("浏览器不允许复制，请选中密钥手动保存。");
            }
          }}
        >
          {copied ? <Check size={16} /> : <Copy size={16} />}{" "}
          {copied ? "已复制" : "复制密钥"}
        </Button>
        {error && <ErrorNote>{error}</ErrorNote>}
      </div>
      <div className="modal-footer">
        <Button variant="secondary" onClick={onClose}>
          我已保存
        </Button>
      </div>
    </Modal>
  );
}
