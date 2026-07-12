import { MotionConfig } from "motion/react";
import { RouterProvider } from "react-router/dom";

import { router } from "./router";

export function App() {
  return (
    <MotionConfig reducedMotion="user">
      <RouterProvider router={router} />
    </MotionConfig>
  );
}
