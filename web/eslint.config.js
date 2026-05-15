import js from "@eslint/js";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import jsxA11y from "eslint-plugin-jsx-a11y";
import globals from "globals";

// jsx-a11y issues in the legacy CSS-module components are tracked as warnings
// here. They are resolved when those components are replaced with accessible
// primitives, after which these can be promoted back to errors.
const a11yWarn = Object.fromEntries(
  Object.keys(jsxA11y.flatConfigs.recommended.rules).map((rule) => [
    rule,
    "warn",
  ]),
);

export default tseslint.config(
  {
    ignores: [
      "dist/**",
      "node_modules/**",
      ".vite/**",
      "coverage/**",
      "playwright-report/**",
      "test-results/**",
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  jsxA11y.flatConfigs.recommended,
  {
    files: ["**/*.{ts,tsx}"],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: "module",
      globals: { ...globals.browser },
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    plugins: {
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      ...a11yWarn,
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "warn",
      "react-refresh/only-export-components": [
        "warn",
        { allowConstantExport: true },
      ],
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
      "@typescript-eslint/no-explicit-any": "warn",
    },
  },
  {
    files: ["**/*.{test,spec}.{ts,tsx}", "tests/**", "e2e/**"],
    languageOptions: { globals: { ...globals.node } },
  },
);
