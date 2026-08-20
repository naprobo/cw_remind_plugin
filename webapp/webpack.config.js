const path = require('path');

module.exports = {
  entry: './src/index.tsx',
  output: {path: path.resolve(__dirname, 'dist'), filename: 'main.js'},
  module: {rules: [
    {test: /\.tsx?$/, exclude: /node_modules/, use: {loader: 'babel-loader', options: {presets: ['@babel/preset-env', ['@babel/preset-react', {runtime: 'automatic'}], '@babel/preset-typescript']}}},
    {test: /\.css$/, use: ['style-loader', 'css-loader']},
  ]},
  resolve: {extensions: ['.tsx', '.ts', '.js']},
};
