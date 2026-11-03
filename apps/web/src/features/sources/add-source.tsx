import { ArrowRight } from "lucide-react";
import { useApp } from "../../app/state";
import { BrandIcon } from "../../components/brand-icon";
import { Button, Modal } from "../../components/ui";
import { accountProvider } from "../../domain/accounts";
import { accounts } from "../../domain/config";
import { AzurePicker } from "./azure-picker";
import { SourceEditor } from "./source-editor";
export function AddSource({ onClose }: { onClose: () => void }) {
  const { config, save, show, close, navigate } = useApp();
  return (
    <Modal
      title="添加来源"
      description="选择账号，再选择它的资源和模型。"
      fixedHeight
      onClose={onClose}
    >
      <div className="form-body account-choices">
        {accounts(config).length === 0 && (
          <p className="muted">先添加并登录账号，再选择模型来源。</p>
        )}
        {accounts(config).map((a) => (
          <Button
            key={a.id}
            variant="secondary"
            onClick={() =>
              show(
                a.kind === "azure" ? (
                  <AzurePicker account={a} onClose={close} />
                ) : (
                  <SourceEditor
                    config={config}
                    save={save}
                    close={close}
                    account={a.id}
                  />
                ),
              )
            }
          >
            <BrandIcon kind={a.kind} />
            <span>
              <strong>{a.name}</strong>
              <small>
                {a.kind === "github"
                  ? "GitHub Copilot"
                  : a.kind === "azure"
                    ? "Azure Foundry / AOAI"
                    : accountProvider(a.kind) + " API"}
              </small>
            </span>
            <ArrowRight size={16} />
          </Button>
        ))}
        <Button
          variant="ghost"
          onClick={() => {
            close();
            navigate("accounts");
          }}
        >
          添加账号
        </Button>
      </div>
    </Modal>
  );
}
