var gp = require("gulp");
var webpack = require('webpack-stream');

gp.task("webpack", function() {
    return gp.src([
            'src/js/index.js',
            'src/sass/index.scss'
        ])
        .pipe(webpack(require('./webpack.config.js')))
        .pipe(gp.dest('../build/dist/'))
})

gp.task("build", ["webpack"], function() {
    gp.src(['./src/html/*.html'])
        .pipe(gp.dest('../build/dist'))
})

gp.task("run", ["build"], function() {
    gp.watch('src/*.js', function() {
        gulp.run('run');
    });
})