import { useEffect, useState, useRef } from "react";
import { api } from "../../api/transport";
import { type Account } from "../../api/types";
import { useApp, useLoad, useDraftBase } from "../../app/state";
import { Loading } from "../../components/feedback";
import { Button, Editor, Field, Input, Select } from "../../components/ui";
import { generatedID, suggestedName } from "../../domain/config";
import { SourceEditor } from "./source-editor";
export function AzurePicker({
  account,
  onClose,
}: {
  account: Account;
  onClose: () => void;
}) {
  const { save, show, close } = useApp(),
    config = useDraftBase();
  const subs = useLoad<{
    subscriptions: { id: string; name: string; is_default?: boolean }[];
  }>(`accounts/${account.id}/subscriptions`);
  const [subscription, setSubscription] = useState(""),
    [subscriptionFilter, setSubscriptionFilter] = useState(""),
    [resourceFilter, setResourceFilter] = useState(""),
    [resource, setResource] = useState(""),
    [resources, setResources] = useState<
      { id: string; name: string; endpoint: string }[]
    >([]),
    [models, setModels] = useState<string[]>([]),
    [selected, setSelected] = useState<string[]>([]),
    [id, setID] = useState(() => generatedID("azure")),
    [name, setName] = useState(""),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(false);
  const selectedDefault = useRef(false);
  useEffect(() => {
    if (!subs.data || selectedDefault.current) return;
    selectedDefault.current = true;
    const preferred =
      subs.data.subscriptions.find((s) => s.is_default) ||
      (subs.data.subscriptions.length === 1
        ? subs.data.subscriptions[0]
        : undefined);
    if (preferred) void loadResources(preferred.id);
  }, [subs.data]);
  async function loadResources(value: string) {
    setSubscription(value);
    setResource("");
    setModels([]);
    setResources([]);
    setResourceFilter("");
    setError("");
    if (!value) return;
    setLoading(true);
    try {
      const v = await api<{ resources: typeof resources }>(
        `accounts/${account.id}/resources?subscription=${encodeURIComponent(value)}`,
      );
      setResources(v.resources);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  async function loadModels(value: string) {
    setResource(value);
    setModels([]);
    setSelected([]);
    setError("");
    const r = resources.find((r) => r.id === value);
    if (!r) return;
    setName(suggestedName(r.name, config.sources));
    setLoading(true);
    try {
      const v = await api<{ models: { id: string }[] }>(
        `accounts/${account.id}/deployments?subscription=${encodeURIComponent(subscription)}&resource=${encodeURIComponent(value)}`,
      );
      setModels(v.models.map((m) => m.id));
      setSelected(v.models.map((m) => m.id));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }
  return (
    <Editor
      title="添加 Azure 来源"
      description="选择这个账号下需要使用的资源和模型。"
      fixedHeight
      saveLabel="添加来源"
      onClose={onClose}
      onSave={async () => {
        const r = resources.find((r) => r.id === resource);
        if (!r || !selected.length)
          throw Error("请选择资源和至少一个 deployment");
        await save({
          ...config,
          sources: [
            ...config.sources,
            {
              id,
              name,
              kind: "azure-openai",
              auth_mode: "azure-cli",
              account_id: account.id,
              subscription_id: subscription,
              resource_id: resource,
              endpoint: r.endpoint,
              deployments: selected,
            },
          ],
        });
        onClose();
      }}
    >
      <Loading loading={subs.loading || loading} error={subs.error || error} />
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={() =>
          show(
            <SourceEditor
              config={config}
              save={save}
              close={close}
              account={account.id}
            />,
          )
        }
      >
        已有 Endpoint？手动添加
      </Button>
      {(subs.data?.subscriptions.length || 0) > 10 && (
        <Field label="筛选订阅">
          <Input
            value={subscriptionFilter}
            onChange={(e) => setSubscriptionFilter(e.target.value)}
            placeholder="按订阅名称搜索"
          />
        </Field>
      )}
      <Field label="订阅">
        <Select
          value={subscription}
          disabled={loading}
          onChange={(e) => void loadResources(e.target.value)}
          required
        >
          <option value="">选择订阅</option>
          {subs.data?.subscriptions
            .filter(
              (s) =>
                s.id === subscription ||
                s.name.toLowerCase().includes(subscriptionFilter.toLowerCase()),
            )
            .map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
        </Select>
      </Field>
      {resources.length > 10 && (
        <Field label="筛选资源">
          <Input
            value={resourceFilter}
            onChange={(e) => setResourceFilter(e.target.value)}
            placeholder="按资源名称搜索"
          />
        </Field>
      )}
      <Field label="AOAI / Foundry 资源">
        <Select
          value={resource}
          disabled={loading || !subscription}
          onChange={(e) => void loadModels(e.target.value)}
          required
        >
          <option value="">选择资源</option>
          {resources
            .filter(
              (r) =>
                r.id === resource ||
                r.name.toLowerCase().includes(resourceFilter.toLowerCase()),
            )
            .map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
        </Select>
      </Field>
      {resource && (
        <>
          <details className="advanced-options">
            <summary>备注与高级设置</summary>
            <Field label="来源名称">
              <Input
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </Field>
            <Field label="来源 ID">
              <Input
                required
                pattern="[a-z][a-z0-9-]{0,62}"
                value={id}
                onChange={(e) => setID(e.target.value)}
              />
            </Field>
          </details>
          <Field label="模型部署">
            {!loading && models.length === 0 && !error && (
              <p className="muted">
                这个资源尚未发现 OpenAI 模型部署。可以选择其他资源，或在 Azure
                中部署模型后重试。
              </p>
            )}
            <div className="checkbox-list">
              {models.map((m) => (
                <label key={m}>
                  <input
                    type="checkbox"
                    checked={selected.includes(m)}
                    onChange={(e) =>
                      setSelected(
                        e.target.checked
                          ? [...selected, m]
                          : selected.filter((v) => v !== m),
                      )
                    }
                  />
                  {m}
                </label>
              ))}
            </div>
          </Field>
        </>
      )}
    </Editor>
  );
}
