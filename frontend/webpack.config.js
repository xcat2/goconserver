const ExtractTextPlugin = require("extract-text-webpack-plugin");
const webpack = require('webpack');

module.exports = {
    output: {
        path: __dirname + '/build/dist/js',
        filename: 'index.js'
    },
    module: {
        rules: [{
                test: /\.scss$/,
                use: ExtractTextPlugin.extract({
                    use: [{
                        loader: "css-loader"
                    }, {
                        loader: "sass-loader"
                    }],
                    fallback: "style-loader"
                })
            },
            {
                test: /\.(png|jpe?g|gif)(\?\S*)?$/,
                use: [{
                    loader: 'file-loader',
                    options: {
                        name: '[name].[ext]',
                        outputPath: '/'
                    }
                }]
            }
        ]
    },
    plugins: [
        new ExtractTextPlugin({
            filename: 'index.css',
        }),
        new webpack.ProvidePlugin({
            $: 'jquery'
        })
    ],
}