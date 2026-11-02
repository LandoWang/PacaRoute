import * as DialogPrimitive from "@radix-ui/react-dialog";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";
import { Loader2, X } from "lucide-react";
import {
  useState,
  createContext,
  useContext,
  useId,
  type ComponentProps,
  type ReactNode,
  type FormEvent,
} from "react";
export const cn = (...values: ClassValue[]) => twMerge(clsx(values));
const buttonVariants = cva("button", {
  variants: {
    variant: {
      default: "button-primary",
      secondary: "button-secondary",
      ghost: "button-ghost",
      destructive: "button-danger",
    },
    size: { default: "", sm: "button-sm", icon: "button-icon" },
  },
  defaultVariants: { variant: "default", size: "default" },
});
export function Button({
  className,
  variant,
  size,
  asChild = false,
  ...props
}: ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot : "button";
  return (
    <Comp
      className={cn(buttonVariants({ variant, size }), className)}
      {...props}
    />
  );
}
const FieldContext = createContext<
  { label: string; hint?: string } | undefined
>(undefined);
export function Input(props: ComponentProps<"input">) {
  const field = useContext(FieldContext);
  return (
    <input
      aria-labelledby={field?.label}
      aria-describedby={field?.hint}
      {...props}
      className={cn("input", props.className)}
    />
  );
}
export function Select(props: ComponentProps<"select">) {
  const field = useContext(FieldContext);
  return (
    <select
      aria-labelledby={field?.label}
      aria-describedby={field?.hint}
      {...props}
      className={cn("input", props.className)}
    />
  );
}
export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  const id = useId();
  return (
    <FieldContext.Provider
      value={{ label: id, hint: hint ? id + "-hint" : undefined }}
    >
      <div className="field">
        <span id={id}>{label}</span>
        {children}
        {hint && <small id={id + "-hint"}>{hint}</small>}
      </div>
    </FieldContext.Provider>
  );
}
export function Badge({
  children,
  tone = "neutral",
}: {
  children: ReactNode;
  tone?: "mint" | "violet" | "amber" | "neutral";
}) {
  return <span className={"badge badge-" + tone}>{children}</span>;
}
export function Panel({
  title,
  description,
  actions,
  children,
  className,
}: {
  title?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={cn("panel", className)}>
      {title && (
        <div className="panel-heading">
          <div>
            <h2>{title}</h2>
            {description && <p>{description}</p>}
          </div>
          {actions}
        </div>
      )}
      {children}
    </section>
  );
}
export function Empty({
  title,
  children,
}: {
  title: string;
  children?: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-mark">◇</div>
      <h3>{title}</h3>
      <p>{children}</p>
    </div>
  );
}
export function ErrorNote({ children }: { children: ReactNode }) {
  return (
    <div role="alert" className="error-note">
      {children}
    </div>
  );
}
export function Modal({
  title,
  description,
  children,
  onClose,
  wide = false,
  fixedHeight = false,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  onClose: () => void;
  wide?: boolean;
  fixedHeight?: boolean;
}) {
  return (
    <DialogPrimitive.Root open onOpenChange={(v) => !v && onClose()}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="modal-overlay" />
        <DialogPrimitive.Content
          className={cn(
            "modal",
            wide && "modal-wide",
            fixedHeight && "modal-fixed-height",
          )}
          onInteractOutside={(e) => e.preventDefault()}
          {...(description ? {} : { "aria-describedby": undefined })}
        >
          <div className="modal-heading">
            <div>
              <DialogPrimitive.Title>{title}</DialogPrimitive.Title>
              {description && (
                <DialogPrimitive.Description>
                  {description}
                </DialogPrimitive.Description>
              )}
            </div>
            <DialogPrimitive.Close asChild>
              <Button variant="ghost" size="icon" aria-label="关闭">
                <X size={18} />
              </Button>
            </DialogPrimitive.Close>
          </div>
          {children}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}
export function Editor({
  title,
  description,
  children,
  onClose,
  onSave,
  wide = false,
  fixedHeight = false,
  saveLabel = "保存",
}: {
  title: string;
  description?: string;
  children: ReactNode;
  onClose: () => void;
  onSave: () => Promise<void>;
  wide?: boolean;
  fixedHeight?: boolean;
  saveLabel?: string;
}) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await onSave();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal
      title={title}
      description={description}
      wide={wide}
      fixedHeight={fixedHeight}
      onClose={() => !busy && onClose()}
    >
      <form onSubmit={submit}>
        <div className="modal-scroll">
          <fieldset disabled={busy} className="form-body">
            {children}
          </fieldset>
        </div>
        {error && <ErrorNote>{error}</ErrorNote>}
        <div className="modal-footer">
          <Button
            type="button"
            variant="secondary"
            disabled={busy}
            onClick={onClose}
          >
            取消
          </Button>
          <Button disabled={busy}>
            {busy && <Loader2 className="spin" size={16} />} {saveLabel}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
