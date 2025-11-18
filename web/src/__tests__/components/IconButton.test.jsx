import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { vi } from "vitest";
import IconButton from "../../components/ui/IconButton";

describe("IconButton", () => {
  it("renders icon and responds to clicks", () => {
    const onClick = vi.fn();
    render(
      <IconButton
        icon={<span data-testid="ic">X</span>}
        onClick={onClick}
        title="btn"
      />,
    );
    const btn = screen.getByTitle("btn");
    expect(screen.getByTestId("ic")).toBeInTheDocument();
    fireEvent.click(btn);
    expect(onClick).toHaveBeenCalled();
  });

  it("does not call onClick when disabled", () => {
    const onClick = vi.fn();
    render(<IconButton icon={<span>X</span>} onClick={onClick} disabled />);
    const btn = screen.getByRole("button");
    fireEvent.click(btn);
    expect(onClick).not.toHaveBeenCalled();
  });

  it("applies inline styles passed via style prop", () => {
    const onClick = vi.fn();
    render(
      <IconButton
        icon={<span>X</span>}
        onClick={onClick}
        style={{ marginLeft: 10 }}
      />,
    );
    const btn = screen.getByRole("button");
    // style is applied inline; marginLeft should be "10px"
    expect(btn.style.marginLeft).toBe("10px");
  });
});
