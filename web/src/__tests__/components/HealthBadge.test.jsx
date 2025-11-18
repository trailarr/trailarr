import React from "react";
import { render, screen } from "@testing-library/react";
import HealthBadge from "../../components/ui/HealthBadge";

describe("HealthBadge", () => {
  it("renders null when count is zero or negative", () => {
    const { container: c1 } = render(<HealthBadge count={0} />);
    expect(c1.firstChild).toBeNull();
    const { container: c2 } = render(<HealthBadge count={-1} />);
    expect(c2.firstChild).toBeNull();
  });

  it("renders number and aria-label for positive counts", () => {
    render(<HealthBadge count={5} />);
    const el = screen.getByLabelText("5 health issues");
    expect(el).toBeInTheDocument();
    expect(el.textContent).toBe("5");
  });

  it("renders orange badge when there are warnings (no errors)", () => {
    render(<HealthBadge count={3} hasError={false} />);
    const el = screen.getByLabelText("3 health issues");
    expect(el).toBeInTheDocument();
    // background style should be the orange warning color
    const bg = window.getComputedStyle(el).backgroundColor;
    expect(bg).toBe("rgb(245, 158, 11)");
  });

  it("renders red badge when there are errors", () => {
    render(<HealthBadge count={2} hasError={true} />);
    const el = screen.getByLabelText("2 health issues");
    expect(el).toBeInTheDocument();
    const bg2 = window.getComputedStyle(el).backgroundColor;
    expect(bg2).toBe("rgb(239, 68, 68)");
  });

  it("caps display at 9+", () => {
    render(<HealthBadge count={12} />);
    const el = screen.getByLabelText("12 health issues");
    expect(el.textContent).toBe("9+");
  });
});
