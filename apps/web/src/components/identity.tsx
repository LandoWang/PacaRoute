import { Input, Field } from "./ui";
// Browsers compile `pattern` with the `v` flag, where an unescaped trailing
// `-` in a character class is invalid and silently disables validation.
export const identifier = {
  pattern: "[a-z][a-z0-9\\-]{0,62}",
  title: "小写字母开头，可使用小写字母、数字和短横线",
  required: true,
};
export function Identity({
  id,
  name,
  onID,
  onName,
  existing = false,
}: {
  id: string;
  name: string;
  onID: (v: string) => void;
  onName: (v: string) => void;
  existing?: boolean;
}) {
  return (
    <>
      <Field label="名称">
        <Input value={name} onChange={(e) => onName(e.target.value)} required />
      </Field>
      <details className="advanced-options">
        <summary>高级设置</summary>
        <Field label="内部 ID" hint="已自动生成，通常无需修改">
          <Input
            {...identifier}
            value={id}
            onChange={(e) => onID(e.target.value)}
            disabled={existing}
          />
        </Field>
      </details>
    </>
  );
}
