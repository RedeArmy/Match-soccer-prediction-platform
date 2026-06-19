import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import {
  DepositLoadingSpinner,
  DepositNotFound,
  DepositSuccess,
  DepositFormError,
  DepositBackLink,
  DepositSubmitButton,
  providerLabel,
} from "@/components/deposit/DepositPageState";

// ── DepositLoadingSpinner ─────────────────────────────────────────────────────

describe("DepositLoadingSpinner", () => {
  it("renders an animated spinner element", () => {
    const { container } = render(<DepositLoadingSpinner />);
    const spinner = container.querySelector(".animate-spin");
    expect(spinner).not.toBeNull();
  });
});

// ── DepositNotFound ───────────────────────────────────────────────────────────

describe("DepositNotFound", () => {
  it("renders the message", () => {
    render(<DepositNotFound message="No encontrado" backLabel="Volver" />);
    expect(screen.getByText("No encontrado")).toBeTruthy();
  });

  it("renders the back link with correct label and href", () => {
    render(<DepositNotFound message="msg" backLabel="Ir atrás" />);
    const link = screen.getByRole("link", { name: /ir atrás/i });
    expect(link.getAttribute("href")).toBe("/balance");
  });
});

// ── DepositSuccess ────────────────────────────────────────────────────────────

describe("DepositSuccess", () => {
  it("renders title and description", () => {
    render(
      <DepositSuccess
        title="¡Éxito!"
        desc="Tu depósito fue procesado."
        backLabel="Volver al balance"
      />,
    );
    expect(screen.getByText("¡Éxito!")).toBeTruthy();
    expect(screen.getByText("Tu depósito fue procesado.")).toBeTruthy();
  });

  it("renders the back link pointing to /balance", () => {
    render(
      <DepositSuccess
        title="OK"
        desc="desc"
        backLabel="Regresar"
      />,
    );
    const link = screen.getByRole("link", { name: /regresar/i });
    expect(link.getAttribute("href")).toBe("/balance");
  });
});

// ── DepositFormError ──────────────────────────────────────────────────────────

describe("DepositFormError", () => {
  it("returns null when error is empty string", () => {
    const { container } = render(<DepositFormError error="" />);
    expect(container.firstChild).toBeNull();
  });

  it("renders the error message when non-empty", () => {
    render(<DepositFormError error="Archivo demasiado grande" />);
    expect(screen.getByText("Archivo demasiado grande")).toBeTruthy();
  });
});

// ── DepositBackLink ───────────────────────────────────────────────────────────

describe("DepositBackLink", () => {
  it("renders a link to /balance with the given aria-label", () => {
    render(<DepositBackLink label="Regresar a balance" />);
    const link = screen.getByRole("link", { name: "Regresar a balance" });
    expect(link.getAttribute("href")).toBe("/balance");
  });
});

// ── DepositSubmitButton ───────────────────────────────────────────────────────

describe("DepositSubmitButton", () => {
  it("shows label when not busy", () => {
    render(
      <DepositSubmitButton
        onClick={() => undefined}
        disabled={false}
        busy={false}
        busyLabel="Enviando…"
        label="Enviar"
        icon={<span data-testid="icon" />}
      />,
    );
    expect(screen.getByText("Enviar")).toBeTruthy();
    expect(screen.queryByText("Enviando…")).toBeNull();
  });

  it("shows busyLabel when busy", () => {
    render(
      <DepositSubmitButton
        onClick={() => undefined}
        disabled={true}
        busy={true}
        busyLabel="Enviando…"
        label="Enviar"
        icon={null}
      />,
    );
    expect(screen.getByText("Enviando…")).toBeTruthy();
    expect(screen.queryByText("Enviar")).toBeNull();
  });

  it("calls onClick when clicked and not disabled", () => {
    const handleClick = vi.fn();
    render(
      <DepositSubmitButton
        onClick={handleClick}
        disabled={false}
        busy={false}
        busyLabel="Enviando…"
        label="Enviar"
        icon={null}
      />,
    );
    fireEvent.click(screen.getByRole("button"));
    expect(handleClick).toHaveBeenCalledOnce();
  });

  it("is disabled when disabled prop is true", () => {
    render(
      <DepositSubmitButton
        onClick={() => undefined}
        disabled={true}
        busy={false}
        busyLabel="Enviando…"
        label="Enviar"
        icon={null}
      />,
    );
    expect(screen.getByRole("button")).toBeDisabled();
  });

  it("renders the icon slot", () => {
    render(
      <DepositSubmitButton
        onClick={() => undefined}
        disabled={false}
        busy={false}
        busyLabel="Enviando…"
        label="Enviar"
        icon={<span data-testid="my-icon" />}
      />,
    );
    expect(screen.getByTestId("my-icon")).toBeTruthy();
  });
});

// ── providerLabel ─────────────────────────────────────────────────────────────

describe("providerLabel", () => {
  it("returns Recurrente for recurrente", () => {
    expect(providerLabel("recurrente")).toBe("Recurrente");
  });

  it("returns PayPal for paypal", () => {
    expect(providerLabel("paypal")).toBe("PayPal");
  });

  it("returns PayPal for any unknown provider", () => {
    expect(providerLabel("stripe")).toBe("PayPal");
  });
});
