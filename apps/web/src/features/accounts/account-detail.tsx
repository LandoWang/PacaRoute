import { RefreshCw } from "lucide-react";
import { type Account, type CopilotInfo } from "../../api/types";
import { useLoad } from "../../app/state";
import { Loading } from "../../components/feedback";
import { Button, Modal } from "../../components/ui";
import { CopilotQuotaDetails } from "./copilot-quota";
export function AccountDetail({
  account,
  onClose,
}: {
  account: Account;
  onClose: () => void;
}) {
  const result = useLoad<Record<string, unknown>>(
    `accounts/${account.id}/info`,
  );
  return (
    <Modal title={account.name + " · 账号与额度"} onClose={onClose}>
      <div className="form-body">
        <Loading loading={result.loading} error={result.error} />
        {result.data && (
          <>
            <p>{String(result.data.login || "")}</p>
            {account.kind === "github" ? (
              <CopilotQuotaDetails info={result.data as CopilotInfo} />
            ) : (
              <pre>
                {JSON.stringify(
                  result.data.subscriptions || result.data,
                  null,
                  2,
                )}
              </pre>
            )}
            {account.kind === "github" && (
              <Button
                variant="ghost"
                size="sm"
                disabled={result.loading}
                onClick={result.refresh}
              >
                <RefreshCw size={14} />
                刷新额度
              </Button>
            )}
            <p className="muted">
              账号额度来自上游；本机请求与 token 统计在「用量」中查看。
            </p>
          </>
        )}
      </div>
    </Modal>
  );
}
