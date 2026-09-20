// Karma configuration
const path = require("path");
const webpack = require("webpack");

const DEFAULT_MAP_STYLE =
  "https://basemaps.cartocdn.com/gl/positron-gl-style/style.json";

process.env.CHROME_BIN =
  process.env.CHROME_BIN || process.env.CHROMIUM_BIN || undefined;

module.exports = function(config) {
  config.set({
    basePath: path.resolve(__dirname, ".."),
    frameworks: ["mocha", "webpack"],
    files: [
      {pattern: "web/test.webpack.js", watched: false},
      // deck.gl fetches the icon atlas over HTTP, so it has to be served.
      {pattern: "web/assets/*", included: false, served: true, watched: false}
    ],
    proxies: {
      "/assets/": "/base/web/assets/"
    },
    preprocessors: {
      "web/test.webpack.js": ["webpack", "sourcemap"]
    },
    reporters: ["mocha"],
    mochaReporter: {
      showDiff: true
    },
    port: 9877,
    colors: true,
    logLevel: config.LOG_INFO,
    browserConsoleLogOptions: {
      level: "log",
      format: "%b %T: %m",
      terminal: true
    },
    autoWatch: true,
    browsers: ["ChromeHeadlessNoSandbox"],
    customLaunchers: {
      // The GPU sandbox is unavailable in most CI containers, and deck.gl
      // needs WebGL, which headless Chrome only provides through SwiftShader.
      ChromeHeadlessNoSandbox: {
        base: "ChromeHeadless",
        flags: [
          "--no-sandbox",
          "--disable-dev-shm-usage",
          "--use-gl=swiftshader",
          "--enable-unsafe-swiftshader",
          "--ignore-gpu-blocklist"
        ]
      }
    },
    webpack: {
      mode: "development",
      devtool: "inline-source-map",
      module: {
        rules: [
          {
            test: /\.jsx?$/,
            exclude: /node_modules/,
            use: {loader: "babel-loader"}
          },
          {
            test: /\.s?css$/,
            use: ["style-loader", "css-loader", "sass-loader"]
          },
          {
            test: /\.(woff|woff2|eot|ttf|svg|png)$/,
            type: "asset/resource"
          }
        ]
      },
      resolve: {
        extensions: [".js", ".jsx"]
      },
      plugins: [
        new webpack.EnvironmentPlugin({
          NODE_ENV: "test",
          MAP_STYLE: DEFAULT_MAP_STYLE
        })
      ]
    },
    singleRun: false,
    concurrency: Infinity
  });
};
