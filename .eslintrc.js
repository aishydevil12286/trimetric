module.exports = {
  env: {
    browser: true,
    commonjs: true,
    es6: true,
    mocha: true,
    node: true
  },
  extends: ["eslint:recommended", "plugin:react/recommended"],
  parserOptions: {
    ecmaFeatures: {
      experimentalObjectRestSpread: true,
      jsx: true
    },
    sourceType: "module"
  },
  plugins: ["react"],
  rules: {
    indent: [
      "error",
      2,
      {
        SwitchCase: 1
      }
    ],
    "linebreak-style": ["error", "unix"],
    quotes: [
      "error",
      "double",
      {
        allowTemplateLiterals: true,
        avoidEscape: true
      }
    ],
    "react/prop-types": "off",
    // The app is still on React 16 and uses componentWillReceiveProps and
    // ReactDOM.render. Both still work there; migrating to the new lifecycle
    // and createRoot is a separate job, so flag them without failing lint.
    "react/no-deprecated": "warn"
  }
};
