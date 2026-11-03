import { Plus, KeyRound, Trash2 } from "lucide-react";
import { api } from "../../api/transport";
import { type Account } from "../../api/types";
import { useApp, useDraftBase } from "../../app/state";
import { BrandIcon } from "../../components/brand-icon";
import { Confirm } from "../../components/feedback";
import { Button, Badge, Panel, Empty } from "../../components/ui";
import {
  accountProvider,
  deviceAccount,
  sharedAzure,
} from "../../domain/accounts";
import { accounts, sources } from "../../domain/config";
import { AzurePicker } from "../sources/azure-picker";
import { SourceEditor } from "../sources/source-editor";
import { AccountDetail } from "./account-detail";
import { AccountEditor } from "./account-editor";
import { AccountKey } from "./account-key";
import { DeviceLogin } from "./device-login";
function NewAccountSource({ account }: { account: Account }) {
  const { save, close } = useApp(),
    config = useDraftBase();
  return (
    <SourceEditor
      config={config}
      save={save}
      close={close}
      account={account.id}
    />
  );
}
export function Accounts() {
  const app = useApp(),
    { config, show, close, save, notice } = app;
  const common = { config, save, close };
  return (
    <>
      <div className="page-toolbar">
        <p>账号保存身份，来源选择资源。一个账号可以连接多个模型来源。</p>
        <Button
          onClick={() =>
            show(
              <AccountEditor
                {...common}
                onConnect={(a) =>
                  show(
                    a.kind === "pacaroute" ? (
                      <NewAccountSource account={a} />
                    ) : (
                      <DeviceLogin account={a} onClose={close} />
                    ),
                  )
                }
              />,
            )
          }
        >
          <Plus size={16} />
          添加账号
        </Button>
      </div>
      {accounts(config).length === 0 && (
        <Empty title="还没有账号">
          点击「添加账号」，选择提供方并登录或填写 Key。
        </Empty>
      )}
      <div className="cards-grid">
        {accounts(config).map((a) => (
          <Panel key={a.id}>
            <div className="entity-heading">
              <div
                className={
                  "entity-icon " + (a.kind === "github" ? "violet" : "mint")
                }
              >
                <BrandIcon kind={a.kind} account />
              </div>
              <div>
                <h2>{a.name}</h2>
                <small>{accountProvider(a.kind)}</small>
              </div>
              <Badge>
                {sources(config).filter((s) => s.account_id === a.id).length}{" "}
                个来源
              </Badge>
            </div>
            <div className="account-resources">
              {sources(config)
                .filter((s) => s.account_id === a.id)
                .map((s) => (
                  <div key={s.id}>
                    <BrandIcon kind={s.kind} size={16} />
                    {s.name}
                  </div>
                ))}
            </div>
            <div className="actions">
              {!deviceAccount(a.kind) && (
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() =>
                    show(<AccountKey account={a} onClose={close} />)
                  }
                >
                  <KeyRound size={14} />
                  设置账号密钥
                </Button>
              )}
              {deviceAccount(a.kind) && (
                <>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() =>
                      sharedAzure(a)
                        ? notice(
                            "本机共享 Azure 身份请运行 az login；添加独立 Azure 账号可直接使用 Device Login。",
                          )
                        : show(<DeviceLogin account={a} onClose={close} />)
                    }
                  >
                    <KeyRound size={14} />
                    {sharedAzure(a) ? "复用 Azure CLI" : "Device Login"}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() =>
                      show(<AccountDetail account={a} onClose={close} />)
                    }
                  >
                    {a.kind === "github" ? "账号 / 额度" : "查看账号"}
                  </Button>
                </>
              )}
              <Button
                variant="ghost"
                size="sm"
                onClick={() =>
                  show(
                    a.kind === "azure" ? (
                      <AzurePicker account={a} onClose={close} />
                    ) : (
                      <SourceEditor {...common} account={a.id} />
                    ),
                  )
                }
              >
                添加来源
              </Button>
            </div>
            <div className="entity-footer">
              <span>
                {a.kind === "github"
                  ? a.oauth_client_id
                    ? "自定义 OAuth：自动使用 GitHub 返回的 refresh token"
                    : "设备授权持久保存，Copilot exchange token 自动续期"
                  : sharedAzure(a)
                    ? "使用此服务主机上的 Azure CLI 身份"
                    : "来源可以在服务运行期间添加或移除"}
              </span>
              <div className="actions">
                {deviceAccount(a.kind) && !sharedAzure(a) && (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() =>
                      show(
                        <Confirm
                          title="退出账号？"
                          description="后续请求将无法使用这个账号，相关来源配置会保留。"
                          onClose={close}
                          action={async () => {
                            await api(`accounts/${a.id}/logout`, "POST", {});
                            notice("账号已退出");
                          }}
                        />,
                      )
                    }
                  >
                    退出
                  </Button>
                )}
                {
                  <Button
                    aria-label={"移除 " + a.name}
                    variant="ghost"
                    size="icon"
                    onClick={() =>
                      show(
                        <Confirm
                          title="移除账号？"
                          description="请先移除引用此账号的来源。这个操作只移除账号配置。"
                          onClose={close}
                          action={() =>
                            save({
                              ...config,
                              accounts: config.accounts.filter(
                                (x) => x.id !== a.id,
                              ),
                            })
                          }
                        />,
                      )
                    }
                  >
                    <Trash2 size={15} />
                  </Button>
                }
              </div>
            </div>
          </Panel>
        ))}
      </div>
    </>
  );
}
