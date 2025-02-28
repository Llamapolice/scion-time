# This script will create requests with different versions
# 1. ip nonauth
# 2. ip nts
# 3. scion nonauth
# 4. scion spao
# 5. scion nts
# 6. scion spao nts


cd $SCION_TIME_ROOT

sudo killall timeservice
sudo killall timeservice
sudo killall timeservice

for iter in `seq 1 10`;
do
    for c in 1 2 4 8 16 32 64 128 192 256 320 384 448 512;
    do 
        for i in $(seq 1 $c)
        do 
            sudo USE_MOCK_KEYS=TRUE ./timeservice benchmark -config $SCION_TIME_ROOT/testnet/benchmark/config/ip_nonauth_benchmark.toml &
        done
        sleep 20
        sudo killall timeservice
        sudo killall timeservice
        sudo killall timeservice
        sleep 5
    done
done

for iter in `seq 1 10`;
do
    sudo killall timeservice
    sudo killall timeservice
    sudo killall timeservice

    for c in 1 2 4 8 16 32 64 128 192 256 320 384 448 512;
    do 
        for i in $(seq 1 $c)
        do 
            sudo USE_MOCK_KEYS=TRUE ./timeservice benchmark -config $SCION_TIME_ROOT/testnet/benchmark/config/ip_nts_benchmark.toml &
        done
        sleep 20
        sudo killall timeservice
        sudo killall timeservice
        sudo killall timeservice
        sleep 5
    done

    sudo killall timeservice
    sudo killall timeservice
    sudo killall timeservice

done

echo "Done"