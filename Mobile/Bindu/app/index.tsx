import "@/global.css"
import { Text, View } from "react-native";

export default function App() {
    return (
        <View className="flex-1 items-center justify-center bg-background">
            <Text className="text-xl font-bold text-accent">
                Welcome to Nativewind!
            </Text>
        </View>
    );
}