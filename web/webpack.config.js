const path = require("path");
const webpack = require("webpack");
const CopyPlugin = require("copy-webpack-plugin");
const HtmlWebpackPlugin = require("html-webpack-plugin");
const MiniCssExtractPlugin = require("mini-css-extract-plugin");

// The basemap is a plain MapLibre style URL. CARTO serves these without an
// access token, which is why the app needs no API key to render a map. Point
// MAP_STYLE at any other style to swap basemaps.
const DEFAULT_MAP_STYLE =
  "https://basemaps.cartocdn.com/gl/positron-gl-style/style.json";

// Where the Go API is listening during development. Inside Docker Compose
// this is the api service; outside it, localhost.
const API_TARGET = process.env.API_TARGET || "http://localhost:8080";

module.exports = (env, argv) => {
  const isProduction = argv.mode === "production";

  return {
    devtool: isProduction ? "source-map" : "eval-cheap-module-source-map",
    entry: ["./web/src/index.jsx", "./web/src/index.scss"],
    context: path.resolve(__dirname, ".."),
    output: {
      path: path.resolve(__dirname, "dist"),
      filename: "bundle.js",
      publicPath: "/",
      clean: true
    },
    devServer: {
      historyApiFallback: true,
      host: "0.0.0.0",
      port: 8080,
      allowedHosts: "all",
      static: {directory: path.resolve(__dirname, "dist")},
      proxy: [
        {context: ["/api"], target: API_TARGET},
        {context: ["/ws"], target: API_TARGET, ws: true}
      ]
    },
    module: {
      rules: [
        {
          test: /\.jsx?$/,
          exclude: /node_modules/,
          use: {loader: "babel-loader"}
        },
        {
          test: /\.s?css$/,
          use: [
            isProduction ? MiniCssExtractPlugin.loader : "style-loader",
            "css-loader",
            "sass-loader"
          ]
        },
        {
          test: /\.(woff|woff2|eot|ttf|svg|png)$/,
          type: "asset/resource",
          generator: {filename: "assets/[name][ext]"}
        }
      ]
    },
    plugins: [
      new MiniCssExtractPlugin({filename: "bundle.css"}),
      // Let the plugin inject the script and style tags: webpack splits a
      // vendor chunk out of the bundle, and a hand-written script tag would
      // silently miss it.
      new HtmlWebpackPlugin({
        template: path.resolve(__dirname, "index.html"),
        inject: "body"
      }),
      new CopyPlugin({
        patterns: [
          {from: path.resolve(__dirname, "assets"), to: "assets"},
          {from: path.resolve(__dirname, "privacy_policy.html")},
          {from: path.resolve(__dirname, "terms_and_conditions.html")}
        ]
      }),
      new webpack.EnvironmentPlugin({
        NODE_ENV: isProduction ? "production" : "development",
        MAP_STYLE: DEFAULT_MAP_STYLE
      })
    ],
    resolve: {
      extensions: [".js", ".jsx"]
    },
    performance: {
      // deck.gl and maplibre are large by nature; the warning is noise.
      hints: false
    }
  };
};
