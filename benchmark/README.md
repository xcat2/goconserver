## consoleserver benchmark
This benchmark is to test the rest api performance of consoleserver. It could
be used to compare the performance of bulk interface and the normal one.

## requirements
Please setup the dependency of python library for this benchmark script.
```
pip install eventlet requests futurist
```

## run the script
Start the consoleserver daemon at first, then run the script:
```
python api.py
```

Output:
```
*** bulk_post Elapsed time: 0.032074 ***
*** list Elapsed time: 0.004233 ***
*** show Elapsed time: 3.682043 ***
*** put_on Elapsed time: 3.744994 ***
*** put_off Elapsed time: 3.572432 ***
*** bulk_put_on Elapsed time: 0.045229 ***
*** bulk_put_off Elapsed time: 0.075146 ***
*** bulk_delete Elapsed time: 0.030068 ***
*** post Elapsed time: 3.687321 ***
*** delete Elapsed time: 3.774885 ***
```
